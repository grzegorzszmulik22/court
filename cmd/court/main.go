package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"court/internal/adapter/claude"
	"court/internal/daemon"
	"court/internal/ledger"
)

const version = "0.1.0-m1.3"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "version":
		err = cmdVersion()
	case "replay":
		err = cmdReplay(os.Args[2:])
	case "ledger":
		err = cmdLedger(os.Args[2:])
	case "daemon":
		err = cmdDaemon(os.Args[2:])
	case "gate":
		err = cmdGate(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "court:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  court version
  court replay <ledger.sqlite|ledger.jsonl>
  court ledger append <db> <commission> <actor> <type> <body.json|->
  court ledger build  <db> <ledger.jsonl>
  court ledger export <db>
  court ledger gen    --out DIR [--count N] [--seed S]
  court ledger loadgen <db> [--agents 8] [--tools N] [--commits N] [--seed S]
  court ledger verify FILE...
  court ledger mutate --in FILE --out DIR [--per K] [--seed S] [--kinds a,b,...]
  court ledger goldens --out DIR
  court daemon --data-dir DIR --ledger DB [--admin-token T]
  court gate --url U --world W --commit C [--parents a,b]`)
	os.Exit(2)
}

func cmdVersion() error {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer db.Close()
	var v string
	if err := db.QueryRow("select sqlite_version()").Scan(&v); err != nil {
		return err
	}
	fmt.Printf("court %s (sqlite %s)\n", version, v)
	return nil
}

// --------------------------------------------------------------------------
// court replay
// --------------------------------------------------------------------------

func cmdReplay(args []string) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("replay wants one ledger path")
	}
	path := fs.Arg(0)
	if err := ledger.LoadSchemas(""); err != nil {
		return err
	}
	rep, err := ledger.Replay(path)
	if err != nil {
		return err
	}
	if rep.Verified.Class != ledger.ClassOK {
		fmt.Printf("REPLAY %s FAIL verify=%s seq=%d %s\n",
			path, rep.Verified.Class, rep.Verified.Seq, rep.Verified.Detail)
		return fmt.Errorf("replay of %s failed verification", path)
	}
	for _, m := range rep.Mismatches {
		fmt.Printf("REPLAY %s FAIL %s\n", path, m)
	}
	for _, m := range rep.ReconciledBad {
		fmt.Printf("REPLAY %s FAIL %s\n", path, m)
	}
	if len(rep.Mismatches)+len(rep.ReconciledBad) > 0 {
		return fmt.Errorf("replay of %s diverged", path)
	}
	stored := "derived-checked"
	if !rep.StoredDerived {
		stored = "no-stored-derived"
	}
	fmt.Printf("REPLAY %s PASS events=%d tables=%d reconciled=%d %s\n",
		path, rep.Events, len(rep.Tables), rep.ReconciledChecked, stored)
	return nil
}

// --------------------------------------------------------------------------
// court ledger
// --------------------------------------------------------------------------

func cmdLedger(args []string) error {
	if len(args) < 1 {
		usage()
	}
	switch args[0] {
	case "append":
		return cmdLedgerAppend(args[1:])
	case "build":
		return cmdLedgerBuild(args[1:])
	case "export":
		return cmdLedgerExport(args[1:])
	case "gen":
		return cmdLedgerGen(args[1:])
	case "loadgen":
		return cmdLedgerLoadgen(args[1:])
	case "verify":
		return cmdLedgerVerify(args[1:])
	case "mutate":
		return cmdLedgerMutate(args[1:])
	case "goldens":
		return cmdLedgerGoldens(args[1:])
	case "rejects":
		return cmdLedgerRejects(args[1:])
	}
	return fmt.Errorf("unknown ledger subcommand %q", args[0])
}

func cmdLedgerAppend(args []string) error {
	if len(args) != 5 {
		return fmt.Errorf("ledger append wants <db> <commission> <actor> <type> <body.json|->")
	}
	db, commission, actor, typ, bodyArg := args[0], args[1], args[2], args[3], args[4]
	var raw []byte
	var err error
	if bodyArg == "-" {
		raw, err = readAllStdin()
	} else {
		raw, err = os.ReadFile(bodyArg)
	}
	if err != nil {
		return err
	}
	// Decode the body through the typed registry so validation applies, and
	// reject fields the type does not know.
	typed, ok := ledger.BodyType(typ)
	if !ok {
		return fmt.Errorf("unknown event type %q", typ)
	}
	bv := newBodyOf(typed)
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(bv); err != nil {
		return fmt.Errorf("body does not fit %s: %w", typ, err)
	}
	s, err := ledger.Open(db)
	if err != nil {
		return err
	}
	defer s.Close()
	ev, err := s.Append(commission, actor, typ, bv)
	if err != nil {
		return err
	}
	line, _ := ev.Line()
	fmt.Println(string(line))
	return nil
}

func cmdLedgerBuild(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("ledger build wants <db> <ledger.jsonl>")
	}
	lines, ver := ledger.ReadLedgerFile(args[1])
	if ver.Class != ledger.ClassOK {
		return fmt.Errorf("%s: %s %s", args[1], ver.Class, ver.Detail)
	}
	if err := ledger.LoadSchemas(""); err != nil {
		return err
	}
	if v := ledger.VerifyLineBytes(lines); v.Class != ledger.ClassOK {
		return fmt.Errorf("%s: %s %s", args[1], v.Class, v.Detail)
	}
	s, err := ledger.Open(args[0])
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Import(lines)
}

func cmdLedgerExport(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("ledger export wants <db>")
	}
	s, err := ledger.Open(args[0])
	if err != nil {
		return err
	}
	defer s.Close()
	events, err := s.Events()
	if err != nil {
		return err
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for _, ev := range events {
		line, err := ev.Line()
		if err != nil {
			return err
		}
		w.Write(line)
		w.WriteByte('\n')
	}
	return nil
}

func cmdLedgerGen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	out := fs.String("out", "", "output directory")
	count := fs.Int("count", 10, "ledgers to generate")
	seed := fs.Int64("seed", 1, "base seed")
	size := fs.Int("size", 0, "events per ledger (0 = mixed sizes)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("--out required")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	for i := 0; i < *count; i++ {
		n := *size
		if n <= 0 {
			n = 30 + (i%7)*40 + i%13
		}
		lines, err := ledger.Generate(*seed+int64(i)*7919, n)
		if err != nil {
			return err
		}
		name := filepath.Join(*out, fmt.Sprintf("gen-%05d.jsonl", i))
		if err := writeLines(name, lines); err != nil {
			return err
		}
	}
	return nil
}

func cmdLedgerLoadgen(args []string) error {
	fs := flag.NewFlagSet("loadgen", flag.ContinueOnError)
	agents := fs.Int("agents", 8, "agents")
	tools := fs.Int("tools", 10000, "tool events")
	commits := fs.Int("commits", 500, "world commits")
	seed := fs.Int64("seed", 1, "seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("loadgen wants a db path")
	}
	s, err := ledger.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer s.Close()
	n, err := ledger.GenerateLoadToStore(s, *seed, *agents, *tools, *commits)
	if err != nil {
		return err
	}
	fmt.Printf("LOADGEN %s events=%d\n", fs.Arg(0), n)
	return nil
}

func cmdLedgerVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	files := fs.Args()
	if len(files) == 0 {
		return fmt.Errorf("verify wants files")
	}
	if err := ledger.LoadSchemas(""); err != nil {
		return err
	}
	bad := 0
	for _, f := range files {
		v := ledger.VerifyFile(f)
		if v.Seq >= 0 {
			fmt.Printf("VERIFY %s %s seq=%d %s\n", f, v.Class, v.Seq, v.Detail)
		} else {
			fmt.Printf("VERIFY %s %s\n", f, v.Class)
		}
		if v.Class != ledger.ClassOK {
			bad++
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d file(s) failed verification", bad)
	}
	return nil
}

func cmdLedgerMutate(args []string) error {
	fs := flag.NewFlagSet("mutate", flag.ContinueOnError)
	in := fs.String("in", "", "input ledger (.jsonl)")
	indir := fs.String("indir", "", "directory of ledgers (every *.jsonl, sorted)")
	out := fs.String("out", "", "output directory")
	per := fs.Int("per", 0, "mutants per ledger (0 = one per kind)")
	seed := fs.Int64("seed", 1, "seed")
	kinds := fs.String("kinds", "", "comma-separated kinds (default all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var inputs []string
	switch {
	case *indir != "" && *in == "":
		ents, err := filepath.Glob(filepath.Join(*indir, "*.jsonl"))
		if err != nil {
			return err
		}
		sort.Strings(ents)
		inputs = ents
	case *in != "" && *indir == "":
		inputs = []string{*in}
	default:
		return fmt.Errorf("exactly one of --in or --indir required")
	}
	if *out == "" {
		return fmt.Errorf("--out required")
	}
	if err := ledger.LoadSchemas(""); err != nil {
		return err
	}
	var ks []string
	if *kinds == "" {
		ks = ledger.MutantKinds
	} else {
		ks = strings.Split(*kinds, ",")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	for i, f := range inputs {
		lines, ver := ledger.ReadLedgerFile(f)
		if ver.Class != ledger.ClassOK {
			return fmt.Errorf("%s: %s %s", f, ver.Class, ver.Detail)
		}
		if v := ledger.VerifyLineBytes(lines); v.Class != ledger.ClassOK {
			return fmt.Errorf("input ledger does not verify: %s", v.Detail)
		}
		base := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		fseed := *seed + int64(i)
		if *per <= 0 {
			for _, k := range ks {
				name := filepath.Join(*out, base+"."+k+".jsonl")
				if err := writeMutant(name,
					ledger.Mutate(lines, k, fseed), k); err != nil {
					return err
				}
			}
			continue
		}
		for j := 0; j < *per; j++ {
			k := ks[j%len(ks)]
			name := filepath.Join(*out,
				fmt.Sprintf("%s.%s.%02d.jsonl", base, k, j/len(ks)))
			if err := writeMutant(name,
				ledger.Mutate(lines, k, fseed+int64(j)), k); err != nil {
				return err
			}
		}
	}
	return nil
}

// cmdLedgerRejects writes the rejecting golden vectors: one JSONL record per
// strict-decoding rule, {"name","class","b64"} where b64 is the full mutant
// ledger and class is the verdict a correct verifier must report. Regenerate
// with `court ledger rejects --out test/vectors/ledger/rejects.jsonl`.
func cmdLedgerRejects(args []string) error {
	fs := flag.NewFlagSet("rejects", flag.ContinueOnError)
	out := fs.String("out", "", "output file (.jsonl)")
	seed := fs.Int64("seed", 20261115, "seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("--out required")
	}
	if err := ledger.LoadSchemas(""); err != nil {
		return err
	}
	base, err := ledger.Generate(*seed, 40)
	if err != nil {
		return err
	}
	want := map[string]string{
		"unknown-field-envelope": ledger.ClassEnvelope,
		"dup-key-envelope":       ledger.ClassEnvelope,
		"dup-key-body":           ledger.ClassEnvelope,
		"non-utf8":               ledger.ClassJSON,
		"bad-escape":             ledger.ClassJSON,
		"lone-surrogate-high":    ledger.ClassJSON,
		"lone-surrogate-low":     ledger.ClassJSON,
		"surrogate-low-high":     ledger.ClassJSON,
		"surrogate-high-nonlow":  ledger.ClassJSON,
		"range-number":           ledger.ClassJSON,
		"trailing-data":          ledger.ClassJSON,
	}
	var w strings.Builder
	for i, k := range ledger.MutantKinds {
		cls, isRule := want[k]
		if !isRule {
			continue
		}
		mut := ledger.Mutate(base, k, *seed+int64(i))
		var lb strings.Builder
		for _, l := range mut {
			lb.Write(l)
			lb.WriteByte('\n')
		}
		rec, err := json.Marshal(map[string]string{
			"name": k, "class": cls,
			"b64": base64.StdEncoding.EncodeToString([]byte(lb.String())),
		})
		if err != nil {
			return err
		}
		w.Write(rec)
		w.WriteByte('\n')
	}
	return os.WriteFile(*out, []byte(w.String()), 0o644)
}

// writeMutant writes mutated lines; a "truncate" mutant keeps the file
// without its trailing newline.
func writeMutant(path string, lines [][]byte, kind string) error {
	var b strings.Builder
	for i, l := range lines {
		b.Write(l)
		if kind == "truncate" && i == len(lines)-1 {
			break // no trailing newline on the cut line
		}
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func cmdLedgerGoldens(args []string) error {
	fs := flag.NewFlagSet("goldens", flag.ContinueOnError)
	out := fs.String("out", "", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("--out required")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	goldens := map[string]int64{
		"golden-minimal.jsonl": 3,
		"golden-mixed.jsonl":   300,
		"golden-large.jsonl":   2000,
	}
	var names []string
	for n := range goldens {
		names = append(names, n)
	}
	sort.Strings(names)
	seeds := map[string]int64{
		"golden-minimal.jsonl": 11,
		"golden-mixed.jsonl":   22,
		"golden-large.jsonl":   33,
	}
	for _, n := range names {
		lines, err := ledger.Generate(seeds[n], int(goldens[n]))
		if err != nil {
			return err
		}
		if err := writeLines(filepath.Join(*out, n), lines); err != nil {
			return err
		}
	}
	// golden-unicode: hand-written edge-case strings inside a valid chain.
	ulines, err := unicodeGolden()
	if err != nil {
		return err
	}
	if err := writeLines(filepath.Join(*out, "golden-unicode.jsonl"), ulines); err != nil {
		return err
	}
	return nil
}

// unicodeGolden produces a small chain whose bodies carry non-ASCII, control
// and astral-plane characters, exercising JCS string rules end to end.
func unicodeGolden() ([][]byte, error) {
	lines, err := ledger.Generate(99, 40)
	if err != nil {
		return nil, err
	}
	// Inject unicode strings into the first two tool.event bodies, then
	// rechain (hash/prev) so the vector still verifies.
	n := 0
	for i, l := range lines {
		ev, err := ledger.DecodeEnvelope(l)
		if err != nil {
			return nil, err
		}
		if ev.Type != "tool.event" || n >= 2 {
			continue
		}
		n++
		lines = ledger.PatchBody(lines, i, func(b *ledger.Value) {
			fields, _ := ledger.ObjGetPub(b, "fields")
			paths, _ := ledger.ObjGetPub(fields, "paths")
			paths.Arr = append(paths.Arr,
				strV("données/ünïcode/.go"), strV("emoji/😀.go"))
			cmd, _ := ledger.ObjGetPub(fields, "command")
			cmd.Str = "grep -n 'π≈3.14, 😀, control' x.go"
		})
	}
	return lines, nil
}

func strV(s string) *ledger.Value {
	return &ledger.Value{Kind: ledger.String, Str: s}
}

func newBodyOf(rt reflect.Type) any { return reflect.New(rt).Interface() }

func writeLines(path string, lines [][]byte) error {
	var b strings.Builder
	for _, l := range lines {
		b.Write(l)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// --------------------------------------------------------------------------
// court daemon / court gate
// --------------------------------------------------------------------------

// cmdDaemon runs the endpoint process (spec/02 §2). It is the only writer
// of the ledger; it binds loopback on an OS-chosen port and writes
// <data-dir>/daemon.json with its URL and admin token.
func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	dataDir := fs.String("data-dir", "", "Court data directory")
	ledgerPath := fs.String("ledger", "", "ledger database path")
	adminTok := fs.String("admin-token", "", "admin bearer token (default: generated)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dataDir == "" || *ledgerPath == "" {
		return fmt.Errorf("daemon wants --data-dir and --ledger")
	}
	var appendDelay time.Duration
	if v := os.Getenv("COURT_DAEMON_APPEND_DELAY_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			appendDelay = time.Duration(ms) * time.Millisecond
		}
	}
	store, err := ledger.Open(*ledgerPath)
	if err != nil {
		return err
	}
	d, err := daemon.New(store, daemon.Options{
		DataDir:     *dataDir,
		Policy:      claude.Policy{DataDir: *dataDir},
		AdminToken:  *adminTok,
		AppendDelay: appendDelay,
		Logger:      log.New(os.Stderr, "court daemon: ", 0),
	})
	if err != nil {
		store.Close()
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := d.Start(ctx); err != nil {
		store.Close()
		return err
	}
	fmt.Fprintf(os.Stderr, "court daemon: listening %s data %s\n", d.URL(), *dataDir)
	<-ctx.Done()
	d.Close()
	store.Close()
	return nil
}

// cmdGate posts one commit notification to the gate endpoint. The
// generated post-commit hook execs it, so no curl is required inside a
// world (spec/02 §2).
func cmdGate(args []string) error {
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	url := fs.String("url", "", "gate endpoint URL")
	worldPath := fs.String("world", "", "world working directory")
	commit := fs.String("commit", "", "new HEAD commit")
	parents := fs.String("parents", "", "space-separated parent commits")
	timeout := fs.Duration("timeout", 10*time.Second, "post timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *url == "" || *worldPath == "" || *commit == "" {
		return fmt.Errorf("gate wants --url --world --commit")
	}
	var plist []string
	for _, p := range strings.Fields(*parents) {
		plist = append(plist, p)
	}
	if plist == nil {
		plist = []string{}
	}
	raw, err := json.Marshal(map[string]any{
		"world_path": *worldPath, "commit": *commit, "parents": plist,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", *url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: *timeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("gate post: %s", resp.Status)
	}
	return nil
}

func readAllStdin() ([]byte, error) {
	var b strings.Builder
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 1<<20), 1<<20)
	for s.Scan() {
		b.WriteString(s.Text())
	}
	return []byte(b.String()), s.Err()
}
