// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package backendarch

// docs/reference/modules.md is the map the rulebook sends you to in order to
// place a change — "Read it to place a change; don't guess from the package
// name". A module missing from it makes that instruction a dead end, and the
// page reads complete either way: nothing about a missing row looks different
// from a module that has none to need.
//
// That is under-recognition, the one failure mode a census must not have, and
// it had already happened. Five modules — aiactivity, commissions, dealrooms,
// finance, integrations — existed with no row at all while the page's prose
// counted twenty and the rulebook counted twenty-one against a tree of
// twenty-seven. Three documents, three numbers, none of them the tree's, and
// nothing comparing any of them to it.
//
// So the expectation is derived from the directory listing rather than restated
// here: a module added tomorrow is enrolled the moment its directory exists,
// and there is no list in this file to go short.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const moduleCatalog = "../docs/reference/modules.md"

// catalogRow matches a table row naming a module in the catalog's own spelling,
// `| **name** |`. Anchored at the line start and at the cell boundary so a
// module NAMED in another row's prose — several rows legitimately mention a
// sibling — is not mistaken for a row of its own. That direction matters: a
// prose mention counted as a row is a missing module reported as present.
var catalogRow = regexp.MustCompile(`(?m)^\|\s*\*\*([a-z0-9]+)\*\*\s*\|`)

func TestModuleCatalogCoversEveryModule(t *testing.T) {
	entries, err := os.ReadDir("internal/modules")
	if err != nil {
		t.Fatalf("reading the module tree: %v", err)
	}
	inTree := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			inTree[e.Name()] = true
		}
	}
	if len(inTree) == 0 {
		t.Fatal("no module directories found — this gate is reading a tree shape that is gone")
	}

	page, err := os.ReadFile(moduleCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", moduleCatalog, err)
	}
	inCatalog := map[string]bool{}
	for _, m := range catalogRow.FindAllStringSubmatch(string(page), -1) {
		inCatalog[m[1]] = true
	}
	if len(inCatalog) == 0 {
		t.Fatalf("no module rows parsed out of %s — a census that reads nothing agrees with everything", moduleCatalog)
	}

	for name := range inTree {
		if !inCatalog[name] {
			t.Errorf("internal/modules/%s has no row in %s, so the rulebook's instruction to read that page "+
				"to place a change is a dead end for it — add a row (Module | Owns | Spine | Owns tables | HTTP surface)",
				name, moduleCatalog)
		}
	}
	for name := range inCatalog {
		if !inTree[name] {
			t.Errorf("%s has a row for %q and internal/modules/%s does not exist — a reader sent to place a change "+
				"there finds nothing; delete the row or restore the module", moduleCatalog, name, name)
		}
	}
}

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
func TestNoRulebookSpellsOutACountableTally(t *testing.T) {
	// Number words up to the low thirties, which is the range a count of
	// modules, extension units, principle pages or compose-owned tables can
	// plausibly land in. A tally that outgrows this list is one that has
	// already gone wrong twice, so the ceiling is not the interesting
	// direction. Digits are deliberately not matched: a version, a port, an
	// ADR number and a line ceiling are all legitimately numerals, and the
	// prose tallies this rule is about are all written as words.
	numberWord := regexp.MustCompile(`(?i)\b(?:twenty|thirty)(?:[ -](?:one|two|three|four|five|six|seven|eight|nine))?\b|` +
		`\b(?:ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen)\b`)
	// The nouns whose population is a directory listing or a gated map away,
	// and so must never be quoted as a word.
	countable := regexp.MustCompile(`(?i)bounded capabilit|module|extension unit|first-party unit|principle page|table`)

	for _, path := range []string{"../AGENTS.md", "../CLAUDE.md", moduleCatalog} {
		raw, err := os.ReadFile(path)
		if err != nil {
			// CLAUDE.md may be a symlink to AGENTS.md, or absent in a
			// checkout that only carries one. Neither is this gate's business.
			if os.IsNotExist(err) {
				continue
			}
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

var listItem = regexp.MustCompile(`^(?:[-*]\s|\d+\.\s)`)

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
		if strings.HasPrefix(trimmed, "```") {
			flush()
			inFence = !inFence
			continue
		}
		if inFence {
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
