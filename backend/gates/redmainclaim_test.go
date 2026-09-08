// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// Every `claim:` label the prose tells a session to search for is one
// `.github/labels.yml` declares.
//
// THIS ONE FAILS OPEN, which is why it is worth a gate. The rulebook tells a
// session to run `gh pr list --label "claim: main-red"` before it starts fixing
// a red `main`. Rename the label in the source and leave the prose behind — or
// the reverse — and that command still exits 0. It returns an empty list, which
// reads exactly like "nobody is fixing this", so every session concludes it is
// the first and they all diagnose the same failure at once. The mechanism does
// not break loudly; it silently becomes the race it was built to remove.
//
// The corpus is derived rather than listed: any tracked Markdown file naming a
// `claim:` label is a subject. So a third page that starts telling sessions to
// search for one is covered the day it is written, without this file learning
// its name — and a page naming a label that does not exist fails here rather
// than at 3am in somebody's session.
//
// The second case is the other direction, and it is the one a rename gets
// right by accident: the rulebook must go on carrying the instruction at all. A
// claim label nothing tells anyone to look for is a lock nobody takes.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// claimSpan matches a claim label as prose writes one — inside a fenced span,
// so a sentence merely discussing claims is not read as naming a label.
var claimSpan = regexp.MustCompile("`(claim: [a-z][a-z-]*)`")

// rulebookPath is the one file that must carry the instruction, because it is
// the file every session reads before it does anything.
var rulebookPath = filepath.Join(repoRoot, "AGENTS.md")

// lookupCommand matches the search the rulebook tells a session to run, inside
// one fenced span, with the label it searches for captured.
//
// The command and not a bare mention of the label: a rulebook that discusses
// claims in prose while no longer telling anyone what to RUN leaves every
// session to invent the query or skip it, and a gate satisfied by the words
// `claim: main-red` appearing somewhere would call that fine. `[^`+"`"+`]*` keeps the
// match inside a single span so two unrelated sentences cannot combine into
// one, and `(?s)` lets the span wrap a line, which prose of this width does.
var lookupCommand = regexp.MustCompile("(?s)`(gh pr list[^`]*--label \"(claim: [a-z][a-z-]*)\"[^`]*)`")

func TestEveryClaimLabelTheProseNamesIsDeclared(t *testing.T) {
	t.Parallel()

	declared := declaredLabels(t)
	named := claimLabelsNamedInProse(t)

	if len(named) == 0 {
		t.Fatal("no page names a `claim:` label, so this gate measures nothing — " +
			"either the claim mechanism was removed and this file should go with it, " +
			"or the prose stopped spelling the label in a fenced span and the search " +
			"instruction no longer says what to search for")
	}
	for label, pages := range named {
		if !slices.Contains(declared, label) {
			t.Errorf("%s tells a session to look for %q, which .github/labels.yml does not declare.\n"+
				"\tThe search still exits 0 and returns nothing, which reads as "+
				"'nobody is fixing this' — so every session starts diagnosing the same "+
				"failure. Add the label to the source, or fix the spelling here.",
				strings.Join(pages, ", "), label)
		}
	}
}

func TestTheRulebookStillTellsSessionsToRunTheClaimLookup(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(rulebookPath)
	if err != nil {
		t.Fatalf("reading the rulebook: %v", err)
	}
	found := lookupCommand.FindStringSubmatch(string(body))
	if found == nil {
		t.Fatal("AGENTS.md carries no `gh pr list ... --label \"claim: ...\"` command.\n" +
			"\tThe label can exist and the draft pull requests can be opened, and it " +
			"still buys nothing: a session stands down only if the rulebook it reads " +
			"first tells it what to RUN. Prose about claims with no command left in it " +
			"is the shape this misses — so if the mechanism is being retired, retire " +
			"the label and this gate in the same change.")
	}
	if declared := declaredLabels(t); !slices.Contains(declared, found[2]) {
		t.Errorf("the rulebook's lookup searches for %q, which .github/labels.yml does not declare.\n"+
			"\tIt exits 0 and returns nothing, which reads as 'nobody is fixing this'.",
			found[2])
	}
}

// TestEveryDeclaredClaimLabelIsOneSomethingTellsSessionsToLookFor is the other
// direction, and the one this file's own docstring promised before it held it:
// a label declared and searched for by nobody is a lock nobody takes. The
// source is the corpus, so a second claim label added tomorrow is covered
// without this file learning its name.
func TestEveryDeclaredClaimLabelIsOneSomethingTellsSessionsToLookFor(t *testing.T) {
	t.Parallel()

	var claims []string
	for _, label := range declaredLabels(t) {
		if strings.HasPrefix(label, "claim: ") {
			claims = append(claims, label)
		}
	}
	if len(claims) == 0 {
		t.Fatal("the source declares no `claim:` label, so this gate measures nothing — " +
			"if the mechanism was retired, this file should have gone with it")
	}
	named := claimLabelsNamedInProse(t)
	for _, label := range claims {
		if len(named[label]) == 0 {
			t.Errorf("%q is declared and no tracked page tells a session to look for it.\n"+
				"\tA claim label nobody searches for is a lock nobody takes: the session "+
				"holding it believes it has said so, and every other session starts "+
				"the same diagnosis. Name it in AGENTS.md, or retire it from the source.",
				label)
		}
	}
}

// claimLabelsNamedInProse maps each `claim:` label the tree's Markdown names to
// the pages naming it. Both directions read it: one asks whether every name is
// declared, the other whether every declaration is named.
//
// Held by: TestEveryClaimLabelTheProseNamesIsDeclared,
// TestEveryDeclaredClaimLabelIsOneSomethingTellsSessionsToLookFor
func claimLabelsNamedInProse(t *testing.T) map[string][]string {
	t.Helper()
	named := map[string][]string{}
	for _, file := range trackedFiles(t) {
		if file.symlink || !strings.HasSuffix(file.path, ".md") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(repoRoot, file.path))
		if err != nil {
			t.Fatalf("reading %s: %v", file.path, err)
		}
		for _, match := range claimSpan.FindAllStringSubmatch(string(body), -1) {
			if !slices.Contains(named[match[1]], file.path) {
				named[match[1]] = append(named[match[1]], file.path)
			}
		}
	}
	return named
}
