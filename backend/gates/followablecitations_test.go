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
// WHAT IT CANNOT SEE, said rather than left to be assumed.
//
// For a DECISION NUMBER it judges the phrasing, not the prose: a directive word
// in front of a number is a pointer, and a number standing alone is a label. A
// comment that names a decision and then explains nothing is compliant here,
// because no pattern can tell an explanation from a sentence.
//
// And it cannot see a section whose document is never named. `the §4
// arithmetic`, `the §1.3 merge` — around two hundred of these, and they are the
// worst of the family rather than the least of it, because a reader has not
// even a name to search for. They are left because the remedy is not a rewrite
// of the citation: somebody has to know what that section said, and no pattern
// can supply it. The arms below stop the forms that CAN be repaired by whoever
// touches the line.

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
	// The three retired documents whose names carry no punctuation to derive
	// from. `design §6.6`, `formulas §11`, `spec §3` — each names a document
	// that was never in this repository, and a bare word gives the check below
	// nothing to recognise it by, so these three are named.
	//
	// A LIST, where the check below derives, and the reason is worth stating:
	// a name with a dot, a hyphen or a slash is unambiguously a filename and
	// can be resolved against the tree, which is how an unlisted family gets
	// caught. A bare word cannot be told from English — `the §4 arithmetic` is
	// prose — so the derivation has no purchase and the retired names are
	// spelled. `architecture` is deliberately absent: docs/explanation/architecture.md
	// is in this tree, and the retired chapters spell themselves `architecture/03`,
	// which the derived check resolves and refuses on its own.
	{"retired design document", regexp.MustCompile(`\b(design|formulas|spec)\s*§`)},
	// A path INTO the retired specification. Unlike a chapter this needs no
	// section mark: a path is already an instruction to open a file, and
	// `spec/` names a directory that has never been in this repository. The
	// sibling gate bans `specs/` — the plural — and the tree spells it
	// singular, which is how eighty-eight of these sat under a green gate.
	{"specification path", regexp.MustCompile(`(^|[^\w/.-])spec/[a-z0-9]`)},
	// A number from the retired backlog: epics, their numbered items, and the
	// acceptance plan.
	//
	// Matched WHEREVER it appears, for the reason the A-numbers above are. The
	// rulebook blesses a decision number as a label because the record is filed
	// and somebody can ask for it; nothing files an `EP07` or a `B-E11.32`, so
	// the number labels nothing and can only read as a pointer. Where one sits
	// beside a decision record that survived, the record is the label already:
	// `(EP05 / ADR-0006 scrape seam)` becomes `(ADR-0006 scrape seam)` and
	// loses nothing a reader could have used.
	//
	// `AC-W2` is deliberately NOT here. docs/reference/make-targets.md says what
	// it is in the same sentence it names it, so it is a label with its rule
	// beside it — which is the whole of what this gate asks for.
	{"retired backlog number", regexp.MustCompile(
		`\b(B-)?EP?\d{2}(\.\d+[a-z]?)*\b|\bUAT-PLAN-\d\b|\bOP-\d{1,2}\b`)},
}

// chapterCitation is a NAMED document opened at a section: `data-model §12.5`,
// `events.md §7`, `features/07 §11`. The name is captured so it can be resolved
// against this tree.
//
// Required to carry a dot, a hyphen or a slash. That punctuation is what makes
// it a filename rather than English — `the §4 arithmetic` and `the §1.3 merge`
// are prose about a section whose document is never named, and no resolution
// can help a reader there. Those are real and they are not this: see the
// retired-design-document arm above for the bare-word half, and the note on
// what this gate cannot see for the half that names nothing at all.
var chapterCitation = regexp.MustCompile(`\b([A-Za-z][\w.-]*(?:/[\w.-]+)*)\s*§`)

// aDecisionRecord is the one name the derived check hands back untouched.
//
// The rulebook blesses a decision number as a label and the arm at the top of
// this file already judges it by PHRASING — `see ADR-0054` is a pointer,
// `(ADR-0054)` is a filing reference. Resolving it here as an unreachable
// document would overturn that ratified reading from a helper, and for two
// thousand five hundred sites. If the section mark should make a decision
// record a destination too, that is its own change with its own argument.
var aDecisionRecord = regexp.MustCompile(`(?i)^ADR-\d{4}$`)

// treeDocuments collects the Markdown this repository carries, keyed by the
// spellings a citation uses: the path, and the basename with and without its
// extension. A name that misses this set names nothing a reader can open.
//
// DERIVED from the tree rather than listed, which is the whole point. A gate
// carrying a list of retired document names reports the families somebody
// thought of — `data-model` and `features` were listed, and `events.md`,
// `interfaces.md`, `formulas-and-rules`, `ai-operational-spec` and
// `data-semantics` were not, so two hundred sites sat under a green gate. A
// gate that asks the tree cannot miss a family, and it stops being wrong on its
// own the day one of those documents is written here.
func treeDocuments(t testing.TB) map[string]bool {
	t.Helper()
	// `-C ..` for the reason changedFilesAgainstMain uses it: this package runs
	// from backend/, and docs/ — which is most of what a citation legitimately
	// names — is above it. Listed from backend/ the set holds no docs/ page at
	// all and every citation resolves as unfollowable, which is a gate that
	// fails in the loud direction rather than the silent one and is still wrong.
	out, err := exec.Command("git", "-C", "..", "ls-files", "*.md").Output()
	if err != nil {
		t.Fatalf("listing this tree's documents: %v", err)
	}
	docs := map[string]bool{}
	for _, path := range strings.Fields(string(out)) {
		base := filepath.Base(path)
		for _, spelling := range []string{path, base, strings.TrimSuffix(base, ".md")} {
			docs[strings.ToLower(spelling)] = true
		}
	}
	// A tree with no Markdown in it is a broken checkout, and clearing every
	// citation over it is the direction this gate must not fail in.
	if len(docs) == 0 {
		t.Fatal("this tree lists no Markdown at all, so every citation would resolve as unfollowable")
	}
	return docs
}

// citesAnAbsentDocument reports the named documents on this line that are not
// in this tree.
func citesAnAbsentDocument(line string, docs map[string]bool) []string {
	var absent []string
	for _, m := range chapterCitation.FindAllStringSubmatch(line, -1) {
		name := m[1]
		if !strings.ContainsAny(name, ".-/") || aDecisionRecord.MatchString(name) {
			continue
		}
		if !docs[strings.ToLower(name)] && !docs[strings.ToLower(name)+".md"] {
			absent = append(absent, name)
		}
	}
	return absent
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
	docs := treeDocuments(t)
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
			for _, name := range citesAnAbsentDocument(at.text, docs) {
				t.Errorf("%s:%d opens %q at a section, and no such document is in this tree — %s\n\t%s",
					rel, at.line, name, citationRemedy, strings.TrimSpace(at.text))
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

// The arms, read against the spellings the tree actually carries and against
// the prose they must leave alone.
//
// The sweep above is diff-scoped, so on a branch that touches none of these
// lines it reports nothing — and an arm that had stopped matching would report
// nothing in exactly the same way. That is how the listed version of this gate
// cleared six hundred citations while looking healthy. These cases fail
// instead.
func TestEveryCitationArmSeesTheSpellingsTheTreeCarries(t *testing.T) {
	t.Parallel()
	docs := treeDocuments(t)
	flagged := func(line string) bool {
		for _, doc := range unreachableDocument {
			if doc.pattern.MatchString(line) {
				return true
			}
		}
		return len(citesAnAbsentDocument(line, docs)) > 0
	}
	for _, k := range []struct {
		why  string
		line string
		want bool
	}{
		{
			"the numbered chapter path, the family the listed gate missed",
			"// routed the lead (features/07 §11 gate 9), so the rep never sees it", true,
		},
		{"the same document without a path", "// versioned (data-model §4.3) — optimistic concurrency", true},
		{"a retired document that no list named", "// the catalog (events.md §7) and its payloads", true},
		{
			"another, to show the derivation is not two entries",
			"// the seam (interfaces.md §3) hands it across", true,
		},
		{
			"a retired document whose bare name has nothing to derive from",
			"// Weighted value (formulas §6) is computed twice", true,
		},
		{"a path into the specification directory", "// Derived from `spec/data-model.md` (the schema)", true},
		{"a retired epic", "// The Morning-Brief HTTP surface (E05): the home read", true},
		{"a retired backlog item", "// the preference center (B-E11.32). The token resolves to", true},
		{
			"a document that IS in this tree, cited at a section",
			"// the layout rules (architecture.md §2) put modules below compose", false,
		},
		{
			"a decision number as a label, which the rulebook blesses",
			"// single-company invariant (ADR-0091 §8) holds here", false,
		},
		{
			"prose about a section that names no document at all",
			"// invites a client to draw a precision the §4 arithmetic does not claim", false,
		},
		{
			"an ordinary word that happens to precede a section mark",
			"// see the §5 catalog below", false,
		},
		{
			"a published budget id explained where it is named",
			"// AC-W2 workflow trigger->dispatch p95, 200ms objective", false,
		},
	} {
		t.Run(k.why, func(t *testing.T) {
			t.Parallel()
			if got := flagged(k.line); got != k.want {
				t.Errorf("the arms report %v for %q, want %v", got, k.line, k.want)
			}
		})
	}
}
