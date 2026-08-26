// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package backendarch

// A live-probed write of a HELD row locks its subject.
//
// auth.EnsureWritableLive narrows the window between deciding and writing; it
// does not close it. The probe reads a snapshot, the write is a later statement
// of the same transaction, and under READ COMMITTED an archive or an Art. 17
// erasure committing in between lands anyway.
//
// For most of the two dozen live-probed paths the residue is a stale write. For
// a path that goes on to INSERT into a table Art. 17 ERASURE CLEARS, it is not:
// the row it writes is one the erasure had just deleted, restored after the
// installation was told to forget it. Those paths owe auth.LockSubjectLive, and
// which ones they are is derived here from the PII registry rather than kept as
// a list — piiTables already declares which tables an erasure clears, and a
// table added there brings its writers into this census on the same commit.
//
// Reachability rather than a same-function check, because the probe and the
// insert are routinely two frames apart: ai.Record probes and upsertVerdict
// writes. The graph is the one packageCallGraph builds, with its package-level
// statement folding.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

const (
	liveProbe   = "EnsureWritableLive"
	subjectLock = "LockSubjectLive"
)

// unlockedLiveWrites ratifies a live-probed writer of an erasure-cleared table
// that does not hold its subject.
//
// Empty, and that is the finding rather than an oversight: every such writer in
// the tree today takes the lock. The map exists so that adding an entry is a
// visible decision with a reason beside it.
var unlockedLiveWrites = gatekit.Waive(map[string]string{})

func TestALiveProbedWriteOfAHeldRowLocksItsSubject(t *testing.T) {
	defer unlockedLiveWrites.AssertAllMatched(t)

	erasureClears := tablesArticle17Deletes(t)
	if len(erasureClears) < 20 {
		t.Fatalf("derived only %d erasure-deleted tables from the privacy module — the derivation is "+
			"broken, not the erasure", len(erasureClears))
	}

	var findings []string
	probed, locked := 0, 0
	for _, dir := range moduleDirsWith(t, liveProbe) {
		graph := packageCallGraph(t, dir)
		for name, entry := range graph {
			// reads, not calls: both of these are cross-package
			// (auth.EnsureWritableLive), and packageCallGraph records a
			// selector call as an edge only when its base is the receiver.
			// The Sel is an identifier either way, so it lands in reads.
			if !entry.reads[liveProbe] {
				continue
			}
			probed++
			table := firstErasureClearedInsert(graph, name, erasureClears)
			if table == "" {
				continue
			}
			if reaches(graph, name, subjectLock) || unlockedLiveWrites.Waived(t, dir+":"+name) {
				locked++
				continue
			}
			findings = append(findings, fmt.Sprintf("%s:%s writes %s", dir, name, table))
		}
	}

	// A census that judged nothing certifies nothing, and this one has two ways
	// to go quiet: the walk can stop finding live-probed functions, and the
	// reach can stop finding the writes under them.
	if probed < 15 || locked < 3 {
		t.Fatalf("this census saw %d live-probed function(s) of which %d write an erasure-cleared table "+
			"under a held subject; it expects at least 15 and 3, so the walk or the reach has stopped "+
			"working rather than the tree having changed", probed, locked)
	}
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf("these functions probe for a LIVE subject and then write a table Art. 17 erasure clears, "+
			"without holding the subject:\n    %s\n\n"+
			"EnsureWritableLive reads a snapshot; the write is a later statement, and an erasure committing "+
			"between them restores the row it had just deleted. Take auth.LockSubjectLive before the write, "+
			"or ratify the writer in unlockedLiveWrites with the reason its residue is harmless.",
			strings.Join(findings, "\n    "))
	}
}

// moduleDirsWith are the module package directories whose source mentions the
// given identifier, so the graph is built only where it can matter. Derived from
// the tree rather than listed: a module that starts live-probing joins this
// census without anybody editing it.
func moduleDirsWith(t *testing.T, identifier string) []string {
	t.Helper()
	paths, err := filepath.Glob("internal/modules/*/*.go")
	if err != nil {
		t.Fatalf("listing the module sources: %v", err)
	}
	dirs := map[string]bool{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(path) // #nosec G304 -- a *.go path from the trusted module tree
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		if strings.Contains(string(source), identifier) {
			dirs[filepath.ToSlash(filepath.Dir(path))] = true
		}
	}
	out := make([]string, 0, len(dirs))
	for dir := range dirs {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out
}

// tablesArticle17Deletes are the tables the privacy module DELETEs from, read
// out of the module that does the deleting.
//
// From the eraser rather than from piicoverage_test.go's registry, and the
// difference is not cosmetic: consent_doi_token is deleted by Art. 17 and is not
// in that registry, so a corpus derived from the declarations would have missed
// the sharpest case this gate exists for — a bearer capability minted for an
// erased person. A registry records what somebody remembered to declare; the
// statements record what the code does.
func tablesArticle17Deletes(t *testing.T) map[string]bool {
	t.Helper()
	tables := map[string]bool{}
	for _, entry := range packageCallGraph(t, "internal/modules/privacy") {
		for _, statement := range entry.statements {
			for _, m := range deleteRe.FindAllStringSubmatch(statement, -1) {
				tables[m[1]] = true
			}
		}
	}
	return tables
}

// reaches reports whether the function, or anything it calls, names the given
// identifier.
//
// DOWNWARD, where guardedBy walks up. That helper asks whether every route TO a
// function is guarded, which is the question a rename census asks. This one asks
// whether the obligation is discharged somewhere in what this function does,
// which is the question a write owes.
func reaches(graph map[string]*graphFunc, name, identifier string) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(fn string) bool {
		if seen[fn] {
			return false
		}
		seen[fn] = true
		entry, known := graph[fn]
		if !known {
			return false
		}
		if entry.reads[identifier] {
			return true
		}
		for called := range entry.calls {
			if walk(called) {
				return true
			}
		}
		return false
	}
	return walk(name)
}

// firstErasureClearedInsert names a table the function, or anything it calls,
// INSERTs into and Art. 17 erasure clears — or "" when it writes none.
func firstErasureClearedInsert(graph map[string]*graphFunc, name string, erasureClears map[string]bool) string {
	seen := map[string]bool{}
	var walk func(string) string
	walk = func(fn string) string {
		if seen[fn] {
			return ""
		}
		seen[fn] = true
		entry, known := graph[fn]
		if !known {
			return ""
		}
		for _, statement := range entry.statements {
			for _, m := range insertRe.FindAllStringSubmatch(statement, -1) {
				if erasureClears[m[1]] {
					return m[1]
				}
			}
		}
		for called := range entry.calls {
			if table := walk(called); table != "" {
				return table
			}
		}
		return ""
	}
	return walk(name)
}
