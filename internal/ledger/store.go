package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Store is the single-writer SQLite ledger (spec/02 section 6). WAL mode, one
// table `ledger` plus derived tables rebuilt by replay.
type Store struct {
	db   *sql.DB
	conn *sql.Conn // one write connection; BEGIN IMMEDIATE serializes writers
	mu   sync.Mutex
	path string
}

// Open opens (or creates) the ledger at path.
func Open(path string) (*Store, error) {
	if err := LoadSchemas(""); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(wal)&_pragma=synchronous(%s)"+
		"&_pragma=busy_timeout(30000)&_pragma=wal_autocheckpoint(10000)", path, SyncLevel)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, conn: conn, path: path}
	if err := s.initSchema(); err != nil {
		conn.Close()
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return s, nil
}

// SyncLevel is the SQLite synchronous pragma for commits. WAL mode at
// NORMAL is durable across process and OS crashes (every commit's WAL
// frames reach stable storage ordering points); FULL adds per-commit fsync
// for power-loss but costs an order of magnitude in append throughput, so
// the store runs at NORMAL to meet MM1.5.
var SyncLevel = "NORMAL"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS ledger(
  seq        INTEGER PRIMARY KEY,
  prev       TEXT NOT NULL,
  at         TEXT NOT NULL,
  commission TEXT NOT NULL,
  actor      TEXT NOT NULL,
  type       TEXT NOT NULL,
  body       TEXT NOT NULL,
  hash       TEXT NOT NULL
) WITHOUT ROWID;
` + derivedDDL

func (s *Store) initSchema() error {
	_, err := s.conn.ExecContext(context.Background(), schemaSQL)
	return err
}

// Close closes the store.
func (s *Store) Close() error {
	s.conn.Close()
	return s.db.Close()
}

// Head returns the current chain head (seq and hash), or seq=-1 when empty.
func (s *Store) Head() (seq int64, hash string, err error) {
	row := s.conn.QueryRowContext(context.Background(),
		"SELECT seq, hash FROM ledger ORDER BY seq DESC LIMIT 1")
	err = row.Scan(&seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, "", nil
	}
	return seq, hash, err
}

// Append adds one event to the chain in a single immediate transaction. The
// seq and prev are allocated inside the write transaction, so a retried or
// racing append can never reuse a seq that failed (spec/01 section 11.1).
func (s *Store) Append(commission, actor, typ string, body any) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var last error
	for attempt := 0; attempt < 100; attempt++ {
		ev, err := s.appendOnce(commission, actor, typ, body)
		if err == nil {
			return ev, nil
		}
		last = err
		if !isBusy(err) {
			return nil, err
		}
		time.Sleep(time.Duration(1+attempt%10) * time.Millisecond)
	}
	return nil, last
}

func isBusy(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		c := se.Code()
		return c&0xff == sqlite3.SQLITE_BUSY || c&0xff == sqlite3.SQLITE_LOCKED
	}
	msg := err.Error()
	return strings.Contains(msg, "busy") || strings.Contains(msg, "locked")
}

// appendOnce runs the whole append inside BEGIN IMMEDIATE ... COMMIT on the
// dedicated connection. The immediate transaction serializes writers and
// makes the head read below see the newest committed row.
func (s *Store) appendOnce(commission, actor, typ string, body any) (*Event, error) {
	ctx := context.Background()
	// A head read outside the transaction, only to label the fault points
	// below with the upcoming seq; the in-transaction headInTx is the
	// authoritative one.
	pseq, _, _ := s.headInTx()
	fault("begin", pseq+1)
	if _, err := s.conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			s.conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	fault("began", pseq+1)
	seq, prevHash, err := s.headInTx()
	if err != nil {
		return nil, err
	}
	fault("head", seq+1)
	bodyVal, err := BodyFrom(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	if err := ValidateBody(typ, body); err != nil {
		return nil, err
	}
	ev := &Event{
		Seq: seq + 1, Prev: prevHash, At: Now(),
		Commission: commission, Actor: actor, Type: typ, Body: bodyVal,
	}
	if seq < 0 {
		ev.Seq = 0
		ev.Prev = ZeroPrev
	}
	if _, err := ev.ComputeHash(); err != nil {
		return nil, err
	}
	fault("envelope", ev.Seq)
	bodyCanon, err := Canonical(ev.Body)
	if err != nil {
		return nil, err
	}
	fault("pre-insert", ev.Seq)
	if _, err := s.conn.ExecContext(ctx,
		"INSERT INTO ledger(seq,prev,at,commission,actor,type,body,hash) VALUES(?,?,?,?,?,?,?,?)",
		ev.Seq, ev.Prev, ev.At, ev.Commission, ev.Actor, ev.Type, string(bodyCanon), ev.Hash); err != nil {
		return nil, err
	}
	fault("inserted", ev.Seq)
	if err := s.project(ctx, ev); err != nil {
		return nil, err
	}
	fault("projected", ev.Seq)
	fault("commit", ev.Seq)
	if _, err := s.conn.ExecContext(ctx, "COMMIT"); err != nil {
		return nil, err
	}
	committed = true
	return ev, nil
}

// Import loads canonical event lines (an exported JSONL ledger) into the
// store verbatim, projecting derived state. The stream must verify first:
// callers run VerifyLineBytes before importing. Used by `court ledger build`.
func (s *Store) Import(lines [][]byte) error {
	ctx := context.Background()
	if _, err := s.conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			s.conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	for _, l := range lines {
		ev, err := DecodeEnvelope(l)
		if err != nil {
			return err
		}
		bodyCanon, err := Canonical(ev.Body)
		if err != nil {
			return err
		}
		if _, err := s.conn.ExecContext(ctx,
			"INSERT INTO ledger(seq,prev,at,commission,actor,type,body,hash) VALUES(?,?,?,?,?,?,?,?)",
			ev.Seq, ev.Prev, ev.At, ev.Commission, ev.Actor, ev.Type,
			string(bodyCanon), ev.Hash); err != nil {
			return err
		}
		if err := s.project(ctx, ev); err != nil {
			return err
		}
	}
	if _, err := s.conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

// Events reads every event back in seq order.
func (s *Store) Events() ([]*Event, error) {
	rows, err := s.conn.QueryContext(context.Background(),
		"SELECT seq,prev,at,commission,actor,type,body,hash FROM ledger ORDER BY seq")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Event
	for rows.Next() {
		var e Event
		var body string
		if err := rows.Scan(&e.Seq, &e.Prev, &e.At, &e.Commission, &e.Actor,
			&e.Type, &body, &e.Hash); err != nil {
			return nil, err
		}
		if e.Body, err = ParseStrict([]byte(body)); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (s *Store) headInTx() (int64, string, error) {
	row := s.conn.QueryRowContext(context.Background(),
		"SELECT seq, hash FROM ledger ORDER BY seq DESC LIMIT 1")
	var seq int64
	var hash string
	err := row.Scan(&seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, "", nil
	}
	return seq, hash, err
}

// ---------------------------------------------------------------------------
// Fault injection for C1.2.4. COURT_LEDGER_FAULT="point[:seq]" makes the
// process exit(99) the first time the named fault point is reached (for a
// matching seq, or any when seq is omitted). Points: the FaultPoints list
// plus project-<table> points inside the projector.
// ---------------------------------------------------------------------------

var faultSpec = os.Getenv("COURT_LEDGER_FAULT")

// FaultPoints lists the deterministic injection points in one append.
var FaultPoints = []string{
	"begin", "began", "head", "envelope",
	"pre-insert", "inserted", "project", "projected", "commit",
}

func fault(point string, seq int64) {
	if faultSpec == "" {
		return
	}
	wantPoint, wantSeq, _ := strings.Cut(faultSpec, ":")
	if point != wantPoint {
		return
	}
	if wantSeq != "" && wantSeq != "-" && wantSeq != fmt.Sprint(seq) {
		return
	}
	fmt.Fprintf(os.Stderr, "fault: exit at %s seq %d\n", point, seq)
	os.Exit(99)
}
