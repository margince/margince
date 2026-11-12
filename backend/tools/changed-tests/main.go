// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command changed-tests reads a pull request's diff (`git diff -U0`) on stdin.
// It prints the tests the diff adds or changes, and how many times
// scripts/repeat-changed-tests.sh runs each of them.
//
// A Go test is chosen by name. The diff reaches it when it changes the test
// function. It also reaches it when it changes a declaration of the package's
// test files that the test refers to, at any depth. A change to a file that is
// not a test file (a fixture, a test harness package) reaches no test. A vitest
// file is chosen when the diff changes the file.
//
// One line per Go package and lane, or per vitest file:
//
//	go-unit         <count>  backend/<dir>    ^(<test>|<test>)$
//	go-integration  <count>  backend/<dir>    ^(<test>)$
//	vitest          <count>  src/<file>.test.ts
//
// The lines are separated by tabs. What the budget left out goes to stderr.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	root := flag.String("root", ".", "the repository root the diff's paths are relative to")
	timing := flag.String("timing", "", "the integration lane's test times (scripts/integration-test-timing.txt)")
	flag.Parse()
	if err := run(os.Stdin, os.Stdout, os.Stderr, *root, *timing); err != nil {
		fmt.Fprintf(os.Stderr, "changed-tests: %v\n", err)
		os.Exit(2)
	}
}

func run(diff io.Reader, out, notes io.Writer, root, timing string) error {
	changes, err := parseDiff(diff)
	if err != nil {
		return err
	}
	times, err := testTimes(timing)
	if err != nil {
		return err
	}
	tests, err := changedGoTests(root, changes)
	if err != nil {
		return err
	}
	var items []selected
	for _, test := range tests {
		cost := 1.0
		if test.lane == integrationLane {
			cost = integrationCost(times, test.dir, test.name)
		}
		items = append(items, selected{lane: test.lane, group: test.dir, name: test.name, cost: cost, direct: test.direct})
	}
	items = append(items, changedVitestFiles(root, changes)...)
	return writePlan(out, notes, items)
}
