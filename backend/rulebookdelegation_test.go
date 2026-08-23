// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package backendarch

// AGENTS.md is the rulebook and the only copy of it. CLAUDE.md exists because
// Claude Code reads CLAUDE.md and not AGENTS.md, so it imports the rulebook with
// `@AGENTS.md` and adds only what is true of Claude Code and false of the other
// harnesses.
//
// This file holds that arrangement, and the failure it is against is the one
// duplication always has: a rule written into one harness's file binds that
// harness and no other. `cli/craft` feeds the nearest AGENTS.md into its gate
// prompt and never reads CLAUDE.md, so a rule that lands here alone is a rule
// the gate cannot see — and nothing about either file looks wrong when that
// happens.
//
// Two assertions, because the arrangement has two halves that break separately:
// the import can go missing (the rulebook stops reaching Claude at all), and a
// rule section can grow back below it (a second copy, drifting from the day it
// is written).

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const (
	rulebookPath = "../AGENTS.md"
	claudePath   = "../CLAUDE.md"
)

// rulebookImport matches the `@AGENTS.md` import that puts the rulebook in front
// of Claude Code. Claude Code skips imports inside code spans and fenced blocks,
// so a backticked mention does not count and neither does this pattern: the
// match must be at the start of a line with no backtick before it.
var rulebookImport = regexp.MustCompile(`(?m)^@AGENTS\.md\s*$`)

func TestClaudeMdImportsTheRulebook(t *testing.T) {
	claude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read %s: %v", claudePath, err)
	}
	if !rulebookImport.MatchString(string(claude)) {
		t.Errorf("%s does not import the rulebook. Claude Code reads CLAUDE.md and not AGENTS.md, so without a "+
			"line reading exactly `@AGENTS.md` (outside backticks, on its own line) a Claude session gets none of "+
			"the rules. Nothing else in the repository notices: every gate that reads the rulebook reads AGENTS.md.",
			claudePath)
	}
}

// ruleSections are the H2 headings that carry binding rules. They live in
// AGENTS.md, and a copy of any of them in CLAUDE.md is a second answer to one
// question — the exact shape *Reuse before you build* refuses.
//
// The list is a list, and that is worth saying out loud rather than hiding: the
// honest derivation would be "every H2 in AGENTS.md", and it is not used here
// because CLAUDE.md legitimately carries a heading of its own ("## Claude
// Code"), so the gate needs to know which headings are the rulebook's rather
// than merely which exist. What keeps it from going short is that it is checked
// against AGENTS.md below: a name here that AGENTS.md has stopped using fails,
// so the list cannot quietly describe a rulebook that no longer exists.
var ruleSections = []string{
	"## What decides a question",
	"## This repository is public",
	"## Build / test / seed",
	"## Shipping a change",
	"## Layout",
	"## DO NOT TOUCH",
	"## The write shape",
	"## Reuse before you build",
	"## Craftsmanship",
	"## License headers",
	"## Rules learned from the review loop",
}

func TestClaudeMdOnlyImportsTheRulebook(t *testing.T) {
	rulebook := readLines(t, rulebookPath)
	claude := readLines(t, claudePath)

	for _, heading := range ruleSections {
		if !hasHeading(rulebook, heading) {
			t.Errorf("%s no longer carries a %q section, so this gate is describing a rulebook that has moved on. "+
				"Update ruleSections to the headings AGENTS.md actually uses.", rulebookPath, heading)
			continue
		}
		if hasHeading(claude, heading) {
			t.Errorf("%s carries a %q section of its own. That is a second copy of a rule: it binds Claude Code and "+
				"nothing else, and `cli/craft` — which reads AGENTS.md and never CLAUDE.md — cannot see it. Move the "+
				"rule into %s and leave CLAUDE.md to the `@AGENTS.md` import plus what is Claude-Code-specific.",
				claudePath, heading, rulebookPath)
		}
	}
}

// TestTheClaudeHalfStaysSmall is a size ceiling, not a style preference. Every
// line of CLAUDE.md that is not the import is a line no other harness reads, so
// the file growing is the duplication coming back one paragraph at a time —
// under headings this gate's list does not name, which is precisely how it would
// get past the check above.
//
// The number is deliberately generous. It is not a target to sit against; it is
// the point at which "a note about Claude Code" has become "a second rulebook"
// and somebody should have to argue for it in review.
func TestTheClaudeHalfStaysSmall(t *testing.T) {
	const ceiling = 80

	claude := readLines(t, claudePath)
	if n := len(claude); n > ceiling {
		t.Errorf("%s is %d lines, over the %d-line ceiling. Only the `@AGENTS.md` import reaches the other "+
			"harnesses, so content here is content that binds Claude Code alone. A rule belongs in %s; a long "+
			"procedure belongs in a skill or a `.claude/rules/` file with a `paths:` glob, which costs a session "+
			"nothing until it is relevant.", claudePath, n, ceiling, rulebookPath)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Split(string(raw), "\n")
}

// hasHeading reports whether any line opens an H2 whose text starts with prefix,
// so a heading may carry a parenthetical without escaping the match.
func hasHeading(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
