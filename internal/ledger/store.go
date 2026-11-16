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
	// stmts caches statements prepared on conn for the hot append path;
	// every use is under s.mu (a Conn is not safe for concurrent use).
	stmts map[string]*sql.Stmt
	// nextSeqHint labels fault points before the transaction starts; the
	// in-transaction head read is authoritative. -1 until first append.
	nextSeqHint int64
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
	s := &Store{db: db, conn: conn, path: path,
		stmts: map[string]*sql.Stmt{}, nextSeqHint: -1}
	if err := s.initSchema(); err != nil {
		conn.Close()
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return s, nil
}

// SyncLevel is the SQLite synchronous pragma for commits. In WAL mode,
// NORMAL keeps every committed append across a process crash and never
// corrupts the ledger, but after an OS crash or power loss the most recent
// commits may roll back: the chain still verifies, it just ends earlier.
// FULL fsyncs every commit and also survives power loss, at a much lower
// append rate than MM1.5's 2,000 events/s needs. M1.8 decides which level
// an acknowledged hook post requires (its denied list: no acknowledgement
// before the write is durable).
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
	// The upcoming-seq label for the pre-transaction fault points comes
	// from nextSeqHint (one real read on the first append only); the
	// in-transaction headInTx below is the authoritative one.
	if s.nextSeqHint < 0 {
		p, _, _ := s.headInTx()
		s.nextSeqHint = p + 1
	}
	pseq := s.nextSeqHint - 1
	fault("begin", pseq+1)
	if err := s.exec(ctx, "BEGIN IMMEDIATE"); err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			s.exec(ctx, "ROLLBACK")
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
	st, err := s.stmt(ctx,
		"INSERT INTO ledger(seq,prev,at,commission,actor,type,body,hash) VALUES(?,?,?,?,?,?,?,?)")
	if err != nil {
		return nil, err
	}
	if _, err := st.ExecContext(ctx,
		ev.Seq, ev.Prev, ev.At, ev.Commission, ev.Actor, ev.Type, string(bodyCanon), ev.Hash); err != nil {
		return nil, err
	}
	fault("inserted", ev.Seq)
	if err := s.project(ctx, ev); err != nil {
		return nil, err
	}
	fault("projected", ev.Seq)
	fault("commit", ev.Seq)
	if err := s.exec(ctx, "COMMIT"); err != nil {
		return nil, err
	}
	committed = true
	s.nextSeqHint = ev.Seq + 1
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

// stmt returns a prepared statement on the write connection, preparing it
// the first time the same SQL text is seen. Callers hold s.mu; statements
// run inside whatever transaction the connection currently has open, since
// appendOnce manages BEGIN/COMMIT as SQL on this same connection.
func (s *Store) stmt(ctx context.Context, q string) (*sql.Stmt, error) {
	if st, ok := s.stmts[q]; ok {
		return st, nil
	}
	st, err := s.conn.PrepareContext(ctx, q)
	if err != nil {
		return nil, err
	}
	s.stmts[q] = st
	return st, nil
}

// exec runs a no-argument statement through the prepared cache.
func (s *Store) exec(ctx context.Context, q string) error {
	st, err := s.stmt(ctx, q)
	if err != nil {
		return err
	}
	_, err = st.ExecContext(ctx)
	return err
}

func (s *Store) headInTx() (int64, string, error) {
	st, err := s.stmt(context.Background(),
		"SELECT seq, hash FROM ledger ORDER BY seq DESC LIMIT 1")
	if err != nil {
		return -1, "", err
	}
	row := st.QueryRowContext(context.Background())
	var seq int64
	var hash string
	err = row.Scan(&seq, &hash)
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
