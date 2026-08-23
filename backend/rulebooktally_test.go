// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package backendarch

// A rulebook must not spell out a tally of anything the tree can be asked for.
// This is the rule the rulebook already states about extension units — "read
// `extensions/` for the live list rather than trusting this sentence — a list in
// prose goes stale the first time somebody adds a unit, and it reads no
// differently when it has" — applied to the rulebook itself, which had been
// exempting itself from it.
//
// It had gone wrong three ways at once: the rulebooks counted twenty-one modules
// and the module catalog's prose counted twenty, against a tree of twenty-seven.
// Nothing compared any of the three to the tree, and none of the sentences looks
// stale when it is.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const moduleCatalog = "../docs/reference/modules.md"

// TestNoRulebookSpellsOutACountableTally is the other half. Backfilling the
// catalog fixes today's five; a spelled-out tally in prose ("the twenty
// bounded capabilities") goes wrong again the next time somebody adds a
// module, and nothing about the sentence looks stale when it does. The
// rulebook already knows this — it tells you to "read `extensions/` for the
// live list rather than trusting this sentence — a count in prose goes stale
// the first time somebody adds a unit" — and then spelled its own module count
// three paragraphs above.
//
// The rule this holds: name the directory, not the number.
//
// It reads PARAGRAPHS rather than lines, because these files are hard-wrapped
// at about eighty columns and a tally is routinely separated from the noun it
// counts by a newline — "surface for all twenty-one, plus the compose-owned
// tables" is a live example, and the line-at-a-time first draft of this gate
// caught the sibling four lines up and not that one. A gate that reads less
// than the prose does reports the same word for it either way, which is the
// failure this file exists to refuse.
// Number words up to the low thirties, which is the range a count of
// modules, extension units, principle pages or compose-owned tables can
// plausibly land in. A tally that outgrows this list is one that has
// already gone wrong twice, so the ceiling is not the interesting
// direction. Digits are deliberately not matched: a version, a port, an
// ADR number and a line ceiling are all legitimately numerals, and the
// prose tallies this rule is about are all written as words.
//
// The floor is ten because below it the rule stops being one. Running this
// against one..nine over the current tree returns twenty findings and
// almost all of them are correct prose — "one Go module", "three ways",
// "Two sanctioned spine shapes, and ONLY two", "in one package". A closed
// set stated as a number is a rule; a population stated as a number is a
// bug; and at those magnitudes nothing mechanical separates them. Above
// ten, a closed set that small is vanishingly rare and a population is the
// likely reading.
var numberWord = regexp.MustCompile(`(?i)\b(?:twenty|thirty)(?:[ -](?:one|two|three|four|five|six|seven|eight|nine))?\b|` +
	`\b(?:ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen)\b`)

// The nouns whose population is a directory listing or a gated map away,
// and so must never be quoted as a word.
//
// This list is hand-written, which makes it a slice of this gate's own
// subject and so the thing *Reuse before you build* rule 5 warns about.
// It is not derived because the derivation does not exist: "a noun whose
// population the tree can be asked for" is a judgement, not a query — no
// listing distinguishes the modules (countable, and it went wrong) from
// the spine shapes (a closed set of two, deliberately stated as two). So
// it will be short, and the honest consequence is that a tally over a
// noun not named here passes. Add the noun when you meet one; do not
// mistake a green run here for proof that no tally is stale.
var countable = regexp.MustCompile(`(?i)\b(?:bounded capabilit\w*|modules?|extension units?|first-party units?|principle pages?|tables?)\b`)

// tallyHits reports whether a file's prose spells out a tally of a countable
// noun. One implementation: the gate below runs it over the real rulebooks, and
// the markdown table tests run it over fixtures. A test that restated the scan
// would be a second copy of the rule it is checking.
func tallyHits(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, para := range prose(string(raw)) {
		if numberWord.MatchString(para.text) && countable.MatchString(para.text) {
			return true
		}
	}
	return false
}

func TestNoRulebookSpellsOutACountableTally(t *testing.T) {
	for _, path := range rulebookProse(t) {
		// Every path here comes from the walk or is the catalog the rulebook
		// sends a reader to, so all of them exist. An unreadable one is a
		// broken gate, never a clean tree — skipping it would report the same
		// word for a file nobody looked at.
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, para := range prose(string(raw)) {
			if !numberWord.MatchString(para.text) || !countable.MatchString(para.text) {
				continue
			}
			t.Errorf("%s:%d spells out a tally of something the tree can be asked for:\n  %s\n"+
				"Name the directory or the catalog instead of the number — a count in prose goes stale the "+
				"first time somebody adds one, and the sentence looks no different when it has.",
				filepath.Base(path), para.line, para.text)
		}
	}
}

// prosePara is one wrapped block of prose, with the line it starts on so a
// finding points at something a reader can open.
type prosePara struct {
	line int
	text string
}

// listItem matches the markers a markdown list actually uses: `-`, `*`, `+`,
// and an ordered item with either a dot or a bracket. Missing a marker joins
// two unrelated items into one paragraph, which can fuse a number from one
// into a claim about the other.
// fenceMarker matches a fence opener or closer, including one that follows a
// list marker on the same line.
var fenceMarker = regexp.MustCompile("^(?:[-*+]\\s+|\\d+[.)]\\s+)?```")

var listItem = regexp.MustCompile(`^(?:[-*+]\s|\d+[.)]\s)`)

// prose splits markdown into paragraphs: it joins the lines of one wrapped
// block so a tally and the noun it counts are read together, and starts a new
// block at a blank line, a heading, a table row or a list item so two
// unrelated bullets are never joined into a claim neither made.
//
// The join is bounded by the block, and that bound is the point. Unbounded, one
// join swallows a whole section and reports a number from one sentence against
// a noun from another — the same failure a statement-joining scan in this tree
// hit when it swallowed a thirty-line const block. Fenced code is dropped
// entirely: a make target is not prose making a claim.
func prose(src string) []prosePara {
	var out []prosePara
	var buf []string
	start := 0
	inFence := false
	flush := func() {
		if len(buf) > 0 {
			out = append(out, prosePara{line: start, text: strings.Join(buf, " ")})
			buf = nil
		}
	}
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		// A fence may be indented, and may open inside a list item
		// (`- ```text`). Matching the trimmed line alone misses the second,
		// and a missed opener leaves the CLOSING fence to toggle the flag the
		// wrong way — after which real code is read as prose and real prose is
		// skipped, both silently.
		if fenceMarker.MatchString(trimmed) {
			flush()
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		// An indented block (four spaces, or a tab) is code too. It carries no
		// claim, and scanning it can only produce a finding about an example.
		// Only at the START of a block, though: markdown allows a wrapped
		// paragraph's later lines to be indented, and dropping those would
		// split one sentence into two.
		if indented(line) && len(buf) == 0 {
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "|") || listItem.MatchString(trimmed) {
			flush()
			if trimmed == "" {
				continue
			}
		}
		if len(buf) == 0 {
			start = i + 1
		}
		buf = append(buf, trimmed)
	}
	flush()
	return out
}

// rulebookProse is every file whose prose this rule binds: each AGENTS.md in the
// tree, and the module catalog they send a reader to. The CLAUDE.md shims are not
// scanned because they are one import line each and cannot hold a sentence —
// `TestEveryClaudeShimIsNothingButTheImport` is what keeps that true. Derived
// rather than listed, so a directory that grows a rulebook is covered the moment
// it does — the alternative is a list of two paths that stopped describing the
// tree the first time a second rulebook appeared, which is the same defect this
// gate is about.
func rulebookProse(t *testing.T) []string {
	t.Helper()
	out := []string{moduleCatalog}
	for _, dir := range rulebookDirs(t) {
		out = append(out, filepath.Join(dir, "AGENTS.md"))
	}
	return out
}

// indented reports whether a line opens an indented code block — four spaces or
// a tab, which markdown reads as code rather than prose.
func indented(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}

// The tally gate is a markdown parser, and a parser's defects are all of the
// same kind: it reads less than the document does, or more. Each case here is
// one that got through, and each fails in a direction worth naming — a missed
// tally (the gate agrees with a stale number) or a false one (the gate refuses
// correct prose, which is how a gate gets switched off).
func TestTheTallyGateReadsMarkdownCorrectly(t *testing.T) {
	tests := []struct {
		name    string
		md      string
		wantHit bool
		why     string
	}{{
		name:    "a tally split across a wrap",
		md:      "surface for all twenty-one, plus\nthe compose-owned tables",
		wantHit: true,
		why:     "the number and its noun are on different lines; a line-at-a-time scan misses it",
	}, {
		name:    "two unrelated ordered items with a bracket marker",
		md:      "1) there are nineteen of them\n2) the module boundary is enforced",
		wantHit: false,
		why:     "`1)` is a list marker, so these are separate items and neither makes the claim",
	}, {
		name:    "a fenced block that opens inside a list item",
		md:      "- ```text\n  twenty modules\n  ```",
		wantHit: false,
		why:     "the fence opens after a list marker; missing it reads the code as prose",
	}, {
		name:    "an indented code block",
		md:      "Run it:\n\n    twenty modules listed\n",
		wantHit: false,
		why:     "four-space indent is code, and an example is not a claim",
	}, {
		name:    "a countable noun inside a longer word",
		md:      "Twenty stable release notes changed.",
		wantHit: false,
		why:     "`stable` is not `table`; without word boundaries the gate refuses correct prose",
	}, {
		name:    "a real tally on one line",
		md:      "The twenty modules under internal/modules.",
		wantHit: true,
		why:     "the case the gate exists for",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "AGENTS.md")
			if err := os.WriteFile(path, []byte(tc.md), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			if got := tallyHits(path); got != tc.wantHit {
				t.Errorf("hit=%v want=%v — %s\n--- input ---\n%s", got, tc.wantHit, tc.why, tc.md)
			}
		})
	}
}
