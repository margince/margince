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
// version of the same idea is that TOUCHED LINES CARRY THEIR OWN EXPLANATION.
//
// LINES, not files, and the difference is what makes the rule payable. The
// contract is forty thousand lines and carries eighty-eight of these citations:
// scoped by file, adding one field to it would have made that author owe
// eighty-eight rewrites of prose they never read, which is not a bar anybody
// meets — it is a bar somebody disables. Scoped by line, the rule costs what
// the change costs, and the backlog converges as those lines are edited.
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
	"strconv"
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
	for rel, lines := range changed {
		if !citationScanned[filepath.Ext(rel)] || generatedSource(rel) {
			continue
		}
		// This file spells the patterns it bans, so scanning it would fail on
		// its own source. The exact path, not the basename.
		if rel == "backend/gates/followablecitations_test.go" {
			continue
		}
		for _, at := range lines {
			for _, doc := range unreachableDocument {
				if !doc.pattern.MatchString(at.text) {
					continue
				}
				t.Errorf("%s:%d cites a %s as though a reader could open it — %s\n\t%s",
					rel, at.line, doc.name, citationRemedy, strings.TrimSpace(at.text))
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

// touchedLine is one line this branch ADDED, with the number it carries in the
// file as it now stands — which is what a reader is told to go and look at.
type touchedLine struct {
	line int
	text string
}

// changedFilesAgainstMain lists the lines this branch adds, by path, from the
// merge base rather than the tip.
//
// The merge base is the same choice the contract-breaking gate makes and for
// the same reason: against the tip, a line main changed after this branch left
// reads as added HERE, and an author would be asked to clean prose they never
// touched. False negatives are the safe direction for a diff-scoped rule — the
// line is judged the next time somebody edits it.
//
// ADDED lines only. A deletion carries nothing to fix, and a line merely NEAR
// an edit is the file-scoped rule this replaced: the contract is forty thousand
// lines, and owning all of them for one field is a bar that gets disabled
// rather than met.
func changedFilesAgainstMain(t *testing.T) (map[string][]touchedLine, bool) {
	t.Helper()
	base, err := exec.Command("git", "-C", "..", "merge-base", "HEAD", "origin/main").Output()
	if err != nil {
		return nil, false
	}
	// --unified=0 so every hunk is exactly the lines that changed, with no
	// context: context lines are somebody else's prose, and reporting them is
	// the whole defect this shape corrects.
	out, err := exec.Command("git", "-C", "..", "diff", "--unified=0",
		"--no-color", strings.TrimSpace(string(base))).Output()
	if err != nil {
		t.Fatalf("reading the diff: %v", err)
	}
	return addedLines(string(out)), true
}

// hunkHeader is the @@ line, whose second range is where the added lines land
// in the file as it now stands.
var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// addedLines reads a unified diff into the lines each file gained.
func addedLines(diff string) map[string][]touchedLine {
	touched := map[string][]touchedLine{}
	var path string
	next := 0
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ b/"):
			path = strings.TrimPrefix(line, "+++ b/")
		case strings.HasPrefix(line, "+++ /dev/null"):
			path = ""
		case strings.HasPrefix(line, "@@ "):
			if match := hunkHeader.FindStringSubmatch(line); match != nil {
				start, err := strconv.Atoi(match[1])
				if err == nil {
					next = start
				}
			}
		case path != "" && strings.HasPrefix(line, "+"):
			touched[path] = append(touched[path], touchedLine{line: next, text: line[1:]})
			next++
		}
	}
	return touched
}

// The diff reader, run against a diff whose verdict is known.
//
// It is the half that decides WHAT this gate judges, and it fails silently in
// both directions: a reader that returned nothing would pass every branch, and
// one that returned context lines would report prose somebody else wrote — the
// defect that made the file-scoped shape unpayable, arriving again by a
// different route.
func TestTheDiffReaderReportsAddedLinesAndNothingElse(t *testing.T) {
	t.Parallel()
	const diff = `diff --git a/backend/one.go b/backend/one.go
--- a/backend/one.go
+++ b/backend/one.go
@@ -12,0 +13,2 @@ func thing() {
+	// see ADR-0054
+	first()
@@ -40 +42 @@ func other() {
-	old()
+	replaced()
diff --git a/backend/gone.go b/backend/gone.go
--- a/backend/gone.go
+++ /dev/null
@@ -1,2 +0,0 @@
-	// see ADR-0054
-	deleted()
`
	got := addedLines(diff)
	if len(got) != 1 {
		t.Fatalf("read %d file(s) from the diff, want 1 — a deleted file carries nothing to fix, "+
			"and attributing its lines to /dev/null is how a reader starts reporting a path that "+
			"does not exist", len(got))
	}
	want := []touchedLine{
		{line: 13, text: "\t// see ADR-0054"},
		{line: 14, text: "\tfirst()"},
		{line: 42, text: "\treplaced()"},
	}
	one := got["backend/one.go"]
	if len(one) != len(want) {
		t.Fatalf("read %d added line(s), want %d: %+v", len(one), len(want), one)
	}
	for i, at := range one {
		if at != want[i] {
			t.Errorf("line %d = %+v, want %+v — the number is where the line lands in the file as "+
				"it now stands, which is what a reader is told to go and look at", i, at, want[i])
		}
	}
	// The removed line cited a record and is not reported: a deletion is the
	// one change that needs no explanation.
	for _, at := range one {
		if strings.Contains(at.text, "old()") || strings.Contains(at.text, "deleted()") {
			t.Errorf("a removed line was reported as touched: %+v", at)
		}
	}
}
