// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The capture insert's columns, placeholders and arguments agree.
//
// The statement is hand-numbered, which the rulebook allows nobody to add and
// this one predates. Nothing in Go or Postgres checks that a column list, its
// $N placeholders and the argument slice stay the same length — a mismatch is
// caught at execution, on a real capture, as an error nobody sees until a
// mailbox stops syncing. This reads the statement itself and counts.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestTheCaptureInsertCountsItsOwnPlaceholders(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("sinkactivity.go")
	if err != nil {
		t.Fatalf("reading the sink: %v", err)
	}
	statement := insertStatement(t, string(source))

	listStart := strings.Index(statement, "(")
	listEnd := strings.Index(statement, ")")
	if listStart < 0 || listEnd < listStart {
		t.Fatal("the capture insert's column list is no longer parenthesised; this gate reads between ( and )")
	}
	columns := strings.Count(statement[listStart:listEnd], ",") + 1

	highest := 0
	for _, match := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(statement, -1) {
		n, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			t.Fatalf("unreadable placeholder %q: %v", match[0], convErr)
		}
		if n > highest {
			highest = n
		}
	}
	if columns != highest {
		t.Errorf("the capture insert names %d columns but its highest placeholder is $%d — "+
			"a mismatch fails at execution, on a real capture, as a sync that stops with no test having said so",
			columns, highest)
	}

	// Every number from 1 up is used. A gap means an argument is being read
	// into the wrong column, which no count of the highest one can see.
	used := map[int]bool{}
	for _, match := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(statement, -1) {
		n, _ := strconv.Atoi(match[1])
		used[n] = true
	}
	for n := 1; n <= highest; n++ {
		if !used[n] {
			t.Errorf("the capture insert skips $%d, so every argument after it lands in the wrong column", n)
		}
	}
}

// insertStatement is the INSERT INTO activity text, from the sink's source.
func insertStatement(t *testing.T, source string) string {
	t.Helper()
	start := strings.Index(source, "INSERT INTO activity (")
	if start < 0 {
		t.Fatal("the sink no longer holds an `INSERT INTO activity (` — if it moved, point this gate at it")
	}
	rest := source[start:]
	end := strings.Index(rest, "ON CONFLICT")
	if end < 0 {
		t.Fatal("the capture insert no longer carries its ON CONFLICT clause; this gate reads up to it")
	}
	return rest[:end]
}
