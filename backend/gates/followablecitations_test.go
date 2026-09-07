// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

//go:build !integration

package gates

// A file this push touched does not send its reader to a document they cannot
// open.
//
// The rulebook draws the line the whole of this gate rests on: a decision
// number MAY appear as a label — `ADR-0054` beside an explanation that stands
// on its own — and may never be cited as though a reader could follow it. The
// first is a filing reference; the second is an instruction to go and read
// something that is not in this tree, and a public contributor meeting it has
// no route past the rule it states.
//
// The sibling gate above bans three LITERAL spellings tree-wide, and those stay
// absolute: a private repository name and a `specs/` path have no legitimate
// use. This one asks about the CLASS, which is why it cannot be absolute — the
// tree carries around 2,500 decision numbers, most of them labels beside real
// prose, and a sweep that removed them all would delete a thousand useful
// filing references while a sweep that judged each one is a review nobody
// performs carefully at that volume.
//
// So it is DIFF-SCOPED, the shape this tree already trusts for its craft bar.
// There the reasoning is "the tree was cleared to zero before the bar was
// armed, so touched code is clean"; here the backlog is real and the honest
// version of the same idea is that TOUCHED CODE CARRIES ITS OWN EXPLANATION.
// The rule becomes true file by file, paid for by work that was happening
// anyway — and a contributor reads the file they are editing, not the tree.
//
// WHAT IT CANNOT SEE, said rather than left to be assumed. It judges the
// PHRASING, not the prose: a directive word in front of a number is a pointer,
// and a number standing alone is a label. A comment that names a decision and
// then explains nothing is compliant here, because no pattern can tell an
// explanation from a sentence. What this stops is the form that leaves a
// reader with an instruction and no way to satisfy it.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// unreachableDocument is what a document outside this tree looks like when it
// is cited. Each is a class rather than a spelling, which is the correction
// this gate exists to make to its neighbour above.
var unreachableDocument = []struct {
	name    string
	pattern *regexp.Regexp
}{
	// A decision record. The number may stand as a label; these forms make it
	// a destination.
	{"decision record", regexp.MustCompile(
		`(?i)\b(see|per|cf\.?|under|following|according to|defined in|specified in|described in|required by|governed by|in accordance with)\s+(the\s+)?ADR-\d{4}`)},
	// An A-number from the retired specification, which is matched WHEREVER it
	// appears rather than only after a directive — the one place this gate does
	// not judge phrasing, and deliberately.
	//
	// The rulebook blesses a DECISION NUMBER as a label: the record is filed
	// somewhere, and the number is how somebody asks for it. It says nothing of
	// the kind about an A-number, whose document was retired entirely — there is
	// nothing to ask for, so the number labels nothing and can only read as a
	// pointer. A reader meeting one has no move at all.
	//
	// Where it is genuinely useful is beside a decision record that survived,
	// and there the record is the label: `(A107/ADR-0061)` becomes `(ADR-0061)`
	// and loses nothing a reader could have used.
	{"retired specification number", regexp.MustCompile(
		`(?i)(\bA\d{2,3}/ADR-\d{4}|\bADR-\d{4}/A\d{2,3}|\b(see|per|cf\.?|under|following|according to)\s+A\d{2,3}\b)`)},
	// A chapter of the retired specification, which is a path into a document
	// that was never in this repository.
	{"specification chapter", regexp.MustCompile(
		`\b(data-model|features|architecture)\.md\s*§`)},
}

// citationRemedy is what an author does about a finding. It is one sentence
// stated in full at every failure rather than a pointer of its own, because a
// gate about unfollowable references that answers with one would be its own
// subject.
const citationRemedy = "write the rule out here, and keep the number beside it as a label if it helps " +
	"somebody filing. What a reader cannot do is open it: the records are not in this tree, so a " +
	"sentence that sends them there leaves them with an instruction and no way to satisfy it"

// citationScanned are the file types this gate reads. Narrower than its
// neighbour's: Markdown under docs/ is excluded because a doc page's job is
// partly to say where a decision came from, and .json and lockfiles carry no
// prose a contributor is asked to follow.
var citationScanned = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".sql": true, ".yaml": true, ".yml": true,
}

func TestEveryTouchedFileExplainsWhatItCites(t *testing.T) {
	t.Parallel()
	changed, ok := changedFilesAgainstMain(t)
	if !ok {
		// A skip is the right answer on a checkout that genuinely has no base
		// — somebody exploring a tarball — and the WRONG one in CI, where a
		// missing base ref is a broken checkout and a skip would disarm this
		// silently. The lane that must never skip says so, the way the
		// contract-breaking and migration-version gates beside it do.
		if os.Getenv("CITATION_SCOPE_REQUIRE_BASE") == "1" {
			t.Fatal("no origin/main to diff against, and CITATION_SCOPE_REQUIRE_BASE=1 — " +
				"fetch the base ref (checkout fetch-depth) rather than letting this gate skip")
		}
		t.Skip("no origin/main to diff against — this gate is diff-scoped by design")
	}
	for _, rel := range changed {
		if !citationScanned[filepath.Ext(rel)] || generatedSource(rel) {
			continue
		}
		// This file spells the patterns it bans, so scanning it would fail on
		// its own source. The exact path, not the basename.
		if rel == "backend/gates/followablecitations_test.go" {
			continue
		}
		body, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil {
			// Deleted, or absent mid-rebase. Not this gate's business.
			continue
		}
		for i, line := range strings.Split(string(body), "\n") {
			for _, doc := range unreachableDocument {
				if !doc.pattern.MatchString(line) {
					continue
				}
				t.Errorf("%s:%d cites a %s as though a reader could open it — %s\n\t%s",
					rel, i+1, doc.name, citationRemedy, strings.TrimSpace(line))
			}
		}
	}
}

// generatedSource answers whether a path is regenerated rather than edited.
// A citation in one of these came from the contract it was generated from, and
// the file to fix is that contract — which this gate scans on its own.
func generatedSource(rel string) bool {
	base := filepath.Base(rel)
	return strings.HasSuffix(base, "_gen.go") ||
		strings.HasPrefix(filepath.ToSlash(rel), "backend/internal/contracts/")
}

// changedFilesAgainstMain lists the paths this branch changes, from the merge
// base rather than the tip.
//
// The merge base is the same choice the contract-breaking gate makes and for
// the same reason: against the tip, a file main changed after this branch left
// reads as changed HERE, and an author would be asked to clean prose they never
// touched. False negatives are the safe direction for a diff-scoped rule —
// the file is judged the next time somebody edits it.
func changedFilesAgainstMain(t *testing.T) ([]string, bool) {
	t.Helper()
	base, err := exec.Command("git", "-C", "..", "merge-base", "HEAD", "origin/main").Output()
	if err != nil {
		return nil, false
	}
	out, err := exec.Command("git", "-C", "..", "diff", "--name-only", "-z",
		strings.TrimSpace(string(base))).Output()
	if err != nil {
		t.Fatalf("listing the changed files: %v", err)
	}
	var changed []string
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel != "" {
			changed = append(changed, rel)
		}
	}
	return changed, true
}

// The judgement, run against lines whose verdict is known.
//
// The gate above is diff-scoped, so on most branches it reads a handful of
// files and passes — which is exactly what a broken pattern also does. These
// cases are what separates the two, and they are also where the rule is
// legible: the DIFFERENCE between a label and a pointer is the whole of this
// gate, and it is easier to read as a table than as three regexes.
func TestALabelPassesAndAPointerDoesNot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		line  string
		cited bool
	}{
		// Pointers: the reader is sent somewhere.
		{"see", "// The write shape, see ADR-0054.", true},
		{"per", "// captured_by comes from the principal per ADR-0054.", true},
		{"under", "// Refused under ADR-0106 §3.", true},
		{"defined in", "// The ladder is defined in ADR-0021.", true},
		{"required by", "-- one audit row per mutation, as required by ADR-0054", true},
		{"case is not the point", "// SEE ADR-0054 for the reasoning.", true},
		// An A-number is matched wherever it appears: its document was retired,
		// so unlike a decision number it labels nothing a reader could ask for.
		{"a retired A-number beside its record", "// The redirect (A107/ADR-0061) is why.", true},
		{"the pair the other way round", "// ADR-0119/A170 decided the page stays.", true},
		{"a chapter of the retired specification", "# Deferred (data-model.md §12): sequences.", true},

		// Labels: the sentence stands on its own and the number files it.
		{"a label after a full explanation", "// Every mutation commits the row, its audit entry and its event in ONE transaction (ADR-0054).", false},
		{"a label leading a full explanation", "// ADR-0054: the write shape is one transaction, so a change and its trail cannot part company.", false},
		{"a bare number in a list of them", "// Supersedes ADR-0031, ADR-0044.", false},
		// The record survives where the A-number does not, so the fix for the
		// pair above is to keep the half a reader can still be told about.
		{"the pair reduced to its surviving half", "// The deployment configuration (ADR-0061): the api bootstraps it at boot.", false},
		{"nothing to cite", "// The status a lead had reached is a fact the trail already holds.", false},
		// The word is only a directive when it introduces the citation. A
		// sentence that happens to contain one must not be a finding, or the
		// gate teaches people to avoid ordinary English near a number.
		{"a directive word elsewhere in the sentence", "// See the store for the ladder; ADR-0021 numbers it.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var matched bool
			for _, doc := range unreachableDocument {
				if doc.pattern.MatchString(tc.line) {
					matched = true
				}
			}
			if matched != tc.cited {
				verdict := "a label the rulebook allows"
				if tc.cited {
					verdict = "a pointer at a document nobody outside the team can open"
				}
				t.Errorf("matched=%v, want %v — this line is %s:\n\t%s", matched, tc.cited, verdict, tc.line)
			}
		})
	}
}
