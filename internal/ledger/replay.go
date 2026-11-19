package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// ReplayReport is the outcome of `court replay` (spec/01 section 11.4).
type ReplayReport struct {
	Events            int
	Verified          Verdict
	Tables            map[string]int
	Mismatches        []string
	ReconciledChecked int
	ReconciledBad     []string
	StoredDerived     bool // false for a JSONL file (nothing to compare against)
	// HeadSeq/HeadHash are the chain head replay printed for the human
	// (CR-004): the pair a reader keeps to verify the tail later.
	HeadSeq  int64
	HeadHash string
}

// Replay verifies a ledger and rebuilds every derived table from the events
// alone. For a SQLite ledger it then compares the rebuilt state with the
// stored derived tables row for row, and recomputes every stored
// decision.reconciled verdict.
func Replay(path string) (*ReplayReport, error) {
	rep := &ReplayReport{Tables: map[string]int{}}
	lines, ver := ReadLedgerFile(path)
	if ver.Class != ClassOK {
		rep.Verified = ver
		return rep, nil
	}
	rep.Verified = VerifyLineBytes(lines)
	if rep.Verified.Class != ClassOK {
		return rep, nil
	}
	events := make([]*Event, 0, len(lines))
	for _, l := range lines {
		ev, err := DecodeEnvelope(l)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	rep.Events = len(events)
	rep.HeadSeq, rep.HeadHash = events[len(events)-1].Seq, events[len(events)-1].Hash

	// Rebuild derived state in a scratch in-memory database.
	mem, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	defer mem.Close()
	ctx := context.Background()
	if _, err := mem.ExecContext(ctx, derivedDDL); err != nil {
		return nil, err
	}
	for _, ev := range events {
		if err := projectMem(mem, ctx, ev); err != nil {
			return nil, err
		}
	}

	stored, isDB := dumpDerivedIfDB(path)
	rep.StoredDerived = isDB
	rebuilt := dumpDerived(mem)
	for _, t := range derivedTables {
		rep.Tables[t.name] = len(rebuilt[t.name])
	}
	if isDB {
		for _, t := range derivedTables {
			a := strings.Join(stored[t.name], "\n")
			b := strings.Join(rebuilt[t.name], "\n")
			if a != b {
				rep.Mismatches = append(rep.Mismatches,
					fmt.Sprintf("table %s differs (stored %d rows, rebuilt %d rows)",
						t.name, len(stored[t.name]), len(rebuilt[t.name])))
			}
		}
	}

	// Recompute every stored decision.reconciled verdict (P2: a verdict must
	// be a deterministic function of the ledger).
	closed := map[string]bool{}
	for _, ev := range events {
		if ev.Type == "commission.closed" {
			closed[ev.Commission] = true
		}
	}
	for _, ev := range events {
		if ev.Type != "decision.reconciled" {
			continue
		}
		rep.ReconciledChecked++
		want, err := recomputeForEvent(mem, ctx, ev, closed[ev.Commission])
		if err != nil {
			return nil, err
		}
		got := map[string]string{
			"sources": canonArr(ev.Body, "sources"),
			"recon":   str(ev.Body, "recon"),
			"flags":   canonArr(ev.Body, "flags"),
		}
		if got["sources"] != want["sources"] || got["recon"] != want["recon"] ||
			got["flags"] != want["flags"] {
			rep.ReconciledBad = append(rep.ReconciledBad,
				fmt.Sprintf("seq %d decision %s: stored %v recomputed %v",
					ev.Seq, str(ev.Body, "decision"), got, want))
		}
	}
	return rep, nil
}

// projectMem applies the projector against a plain *sql.DB handle.
func projectMem(db *sql.DB, ctx context.Context, ev *Event) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return project(conn, ctx, ev)
}

// recomputeForEvent recomputes the verdict a decision.reconciled event must
// carry for the decision it names.
func recomputeForEvent(db *sql.DB, ctx context.Context, ev *Event, closed bool) (map[string]string, error) {
	var kind, cat, writ string
	var petitionI int64
	row := db.QueryRowContext(ctx,
		`SELECT d.kind, d.category, d.petition, COALESCE(w.categories,'')
		 FROM decisions d
		 LEFT JOIN agents a ON a.commission=d.commission AND a.agent=d.agent
		 LEFT JOIN writs w ON w.id=a.writ
		 WHERE d.commission=? AND d.id=?`,
		ev.Commission, str(ev.Body, "decision"))
	err := row.Scan(&kind, &cat, &petitionI, &writ)
	if err == sql.ErrNoRows {
		return map[string]string{"sources": "[]", "recon": "?", "flags": "[]"}, nil
	}
	if err != nil {
		return nil, err
	}
	sources, recon, flags, _ := recomputedVerdict(kind, cat, writ, closed, petitionI != 0)
	return map[string]string{
		"sources": jSources(sources),
		"recon":   recon,
		"flags":   jSources(flags),
	}, nil
}

func jSources(s []string) string {
	v := &Value{Kind: Array}
	for _, x := range s {
		v.Arr = append(v.Arr, &Value{Kind: String, Str: x})
	}
	return string(MustCanonical(v))
}

// dumpDerived serializes every derived table of db into ordered row strings.
func dumpDerived(db *sql.DB) map[string][]string {
	out := map[string][]string{}
	for _, t := range derivedTables {
		rows, err := db.Query("SELECT * FROM " + t.name + " ORDER BY " + t.order)
		if err != nil {
			continue
		}
		cols, _ := rows.Columns()
		var rr []string
		for rows.Next() {
			cells := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cells {
				ptrs[i] = &cells[i]
			}
			if rows.Scan(ptrs...) == nil {
				var sb strings.Builder
				for i, c := range cells {
					if i > 0 {
						sb.WriteByte(31)
					}
					switch v := c.(type) {
					case nil:
						sb.WriteString("NULL")
					case []byte:
						sb.Write(v)
					default:
						fmt.Fprintf(&sb, "%v", v)
					}
				}
				rr = append(rr, sb.String())
			}
		}
		rows.Close()
		out[t.name] = rr
	}
	return out
}

// dumpDerivedIfDB dumps the derived tables of a SQLite ledger path;
// ok=false when path is not a SQLite database.
func dumpDerivedIfDB(path string) (map[string][]string, bool) {
	head := make([]byte, 16)
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	n, _ := f.Read(head)
	f.Close()
	if n != 16 || string(head) != string(sqliteMagic) {
		return nil, false
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(wal)&_pragma=busy_timeout(30000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, false
	}
	defer db.Close()
	return dumpDerived(db), true
}
