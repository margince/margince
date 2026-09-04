// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// "An already-settled answer disowns this contact" is spelled once.
//
// Two paths ask it: the sweep's SELECTOR, which lists the contacts a noise
// verdict already covered, and the per-retraction RECHECK, which re-asks it on
// the retraction's own transaction because the scan and the archive are
// separate transactions. If those two ever asked different questions the sweep
// would archive contacts its own recheck would have spared — and the recheck
// is the thing standing between a withdrawn keep_out and a lost record.
//
// `noiseJudgedStandsSQL` says it is the one spelling. This is what makes that
// true rather than asserted: the predicate's own text, taken from that
// function, must appear nowhere else in the tree.
//
// The needle is DERIVED from the function rather than written here, so a
// reworded predicate cannot leave this gate hunting for a string that no
// longer exists and reporting a pass over nothing.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noiseStandsOwner is the file that owns the spelling.
var noiseStandsOwner = filepath.Join(
	repoRoot, "backend", "internal", "modules", "capture", "strandedcontacts.go")

// noiseStandsMarker locates the distinctive clause inside the owner. Only the
// PREFIX is written here — the clause's own text is read out of the function,
// so this file holds no copy of the predicate to find itself with, and a
// reworded list is compared as it now reads rather than as it once did.
const noiseStandsMarker = "q.kind IN ("

// noiseStandsClause reads the predicate's distinctive line out of its owner.
func noiseStandsClause(t *testing.T, owner string) string {
	t.Helper()
	at := strings.Index(owner, noiseStandsMarker)
	if at < 0 {
		t.Fatalf("%s no longer contains %q, so this gate would search the tree for "+
			"nothing and report a pass having compared nothing. Re-derive the marker "+
			"from noiseJudgedStandsSQL's current text.",
			filepath.Base(noiseStandsOwner), noiseStandsMarker)
	}
	end := strings.Index(owner[at:], ")")
	if end < 0 {
		t.Fatalf("%s opens %q and never closes it", filepath.Base(noiseStandsOwner), noiseStandsMarker)
	}
	return owner[at : at+end+1]
}

func TestTheNoiseJudgedPredicateHasOneSpelling(t *testing.T) {
	t.Parallel()

	owner, err := os.ReadFile(noiseStandsOwner)
	if err != nil {
		t.Fatalf("reading the predicate's owner: %v", err)
	}
	needle := noiseStandsClause(t, string(owner))

	var elsewhere []string
	root := filepath.Join(repoRoot, "backend")
	err = filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil || path == noiseStandsOwner || !strings.HasSuffix(path, ".go") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), needle) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			elsewhere = append(elsewhere, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	for _, path := range elsewhere {
		t.Errorf("%s spells the noise-judged predicate a second time. The sweep's "+
			"selector and its per-retraction recheck build from noiseJudgedStandsSQL "+
			"so they cannot ask different questions; a second spelling is how they "+
			"start to — and the recheck is what stands between a withdrawn keep_out "+
			"and a contact archived anyway.", path)
	}
}
