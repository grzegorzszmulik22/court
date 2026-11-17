// Command validate is the DocketBench and detection-vector validator of
// spec/milestones/M3-detection-and-reconciliation.md (M3.1). It checks the
// rules of spec/04-benchmark.md §2 on every scenario directory, the count
// and shape minimums of M3.1 on the detection vectors, and the coverage
// requirements on the reconciliation traces.
//
// Usage:
//
//	validate scenarios [root]   bench/docketbench/scenarios
//	validate vectors   [root]   test/vectors/detect
//	validate reconcile [root]   test/vectors/reconcile
//	validate all                all three, at their default roots
//
// Flags:
//
//	-j N        scenario workers for repository test runs (default 4)
//	-tests=0|1  run repository test suites (default 1)
//	-quiet      suppress the OK summary lines
//
// Output: one "VIOLATION <code> <where>: <detail>" line per violation;
// exit 0 only when there are none.
package main

import (
	"flag"
	"fmt"
	"os"
)

// options carries the knobs the checks may tune. The rule switches a
// sabotage would target are plain code paths in scenario.go — there is no
// flag that turns a required validation off.
type options struct {
	workers  int
	runTests bool
	quiet    bool
}

type violations struct{ list []string }

func (v *violations) add(code, where, detail string) {
	v.list = append(v.list, fmt.Sprintf("%s %s: %s", code, where, detail))
}

func (v *violations) addf(code, where, format string, a ...any) {
	v.add(code, where, fmt.Sprintf(format, a...))
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: validate scenarios|vectors|reconcile|all [root] [-j N] [-tests=0|1] [-quiet]")
		return 2
	}
	mode, rest := args[0], args[1:]
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	workers := fs.Int("j", 4, "scenario test workers")
	doTests := fs.Bool("tests", true, "run repository test suites")
	quiet := fs.Bool("quiet", false, "suppress OK lines")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	root := fs.Arg(0)

	v := &violations{}
	opts := options{workers: *workers, runTests: *doTests, quiet: *quiet}

	switch mode {
	case "scenarios":
		if root == "" {
			root = "bench/docketbench/scenarios"
		}
		validateScenarios(root, opts, v)
	case "vectors":
		if root == "" {
			root = "test/vectors/detect"
		}
		validateVectors(root, v)
	case "reconcile":
		if root == "" {
			root = "test/vectors/reconcile"
		}
		validateReconcile(root, v)
	case "all":
		validateScenarios("bench/docketbench/scenarios", opts, v)
		validateVectors("test/vectors/detect", v)
		validateReconcile("test/vectors/reconcile", v)
	default:
		fmt.Fprintf(os.Stderr, "validate: unknown mode %q\n", mode)
		return 2
	}

	if len(v.list) > 0 {
		for _, x := range v.list {
			fmt.Println("VIOLATION", x)
		}
		return 1
	}
	if !*quiet {
		fmt.Println("OK", mode)
	}
	return 0
}
