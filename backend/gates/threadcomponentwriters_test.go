// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	threadAssignment     = regexp.MustCompile(`(?i)\bthread_key\s*=`)
	nullThreadAssignment = regexp.MustCompile(`(?i)\bthread_key\s*=\s*NULL\b`)
)

func clearsThreadKey(statement string) bool {
	assignments, clears := 0, 0
	for _, clause := range setClause.FindAllStringSubmatch(withoutSQLNoise(statement), -1) {
		assignments += len(threadAssignment.FindAllString(clause[1], -1))
		clears += len(nullThreadAssignment.FindAllString(clause[1], -1))
	}
	return assignments > 0 && assignments == clears
}

// The ownership census supplies packages; its SQL parser supplies columns.
// Deletions and explicit erasures cannot adopt a retired component.
func TestEveryThreadKeyWriterCoordinatesWithComponentMerges(t *testing.T) {
	t.Parallel()
	writers := 0
	tables := map[string]bool{}
	for dir, writes := range collectTableWrites(t) {
		relevant := false
		for _, write := range writes {
			if slices.Contains(write.cols, "thread_key") {
				relevant = true
			}
		}
		if !relevant {
			continue
		}
		graph := packageCallGraph(t, dir)
		order := threadOrderOf(parsePackageFiles(t, dir))
		for name, entry := range graph {
			for _, statement := range entry.statements {
				for _, write := range sqlWrites(statement) {
					if write.verb == "delete" || !slices.Contains(write.cols, "thread_key") {
						continue
					}
					if write.verb == "update" && clearsThreadKey(statement) {
						continue
					}
					writers++
					tables[write.table] = true
					position := order.writePosition(t, name, statement, write, writes)
					if !order.guardedBefore(name, position, map[string]bool{}) {
						t.Errorf("%s:%s writes %s.thread_key without the shared component coordinator", dir, name, write.table)
					}
				}
			}
		}
	}
	if writers < 10 || len(tables) < 6 {
		t.Fatalf("short thread writer census: %d writes across %d tables", writers, len(tables))
	}
}

func TestThreadKeyErasuresDoNotAdoptAComponent(t *testing.T) {
	t.Parallel()
	for _, statement := range []string{"UPDATE activity SET thread_key = NULL", "UPDATE activity SET thread_key=NULL WHERE id=@id"} {
		if !clearsThreadKey(statement) {
			t.Fatal(statement)
		}
	}
	for _, statement := range []string{
		"UPDATE activity SET thread_key = @to", "INSERT INTO activity (thread_key) VALUES (@key)",
		"UPDATE activity SET thread_key=@to WHERE thread_key=''",
		"UPDATE activity SET body='thread_key=NULL', thread_key=@to",
		"UPDATE activity SET thread_key=NULL; UPDATE activity SET thread_key=@to",
	} {
		if clearsThreadKey(statement) {
			t.Fatal(strings.TrimSpace(statement))
		}
	}
}
