// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// Every label the docs put in front of a session is one `.github/labels.yml`
// declares.
//
// THIS ONE FAILS OPEN, which is why it is worth a gate. The rulebook tells a
// session to run `gh pr list --label "claim: main-red"` before it fixes a red
// `main`, and `gh issue view` before it starts an issue somebody may already
// hold. Rename a label in the source and leave the prose behind — or the
// reverse — and those commands still exit 0. They return an empty list, which
// reads exactly like "nobody is on this", so every session concludes it is the
// first and they all do the same work at once. The mechanism does not break
// loudly; it silently becomes the race it was built to remove.
//
// The corpus is derived rather than listed: any tracked Markdown file handing a
// label to `gh`, as a flag or as a search term, is a subject. So a page that
// starts telling sessions to look for a new label is covered the day it is
// written, without this file learning the name.
//
// AN ARGUMENT, and not every mention. A label named in a sentence is read by a
// human, who notices a name that no longer exists; a label inside a command is
// pasted unread. `claim:` is the exception and is matched in prose too, because
// those four letters name nothing else in this tree — where `status:` also heads
// a JSON field, and `status: shipped` on a docs page is a response body rather
// than an instruction.
//
// The argument is read in every shape a shell accepts it in — quoted either
// way, bare, behind `=`, one member of a comma list, wrapped onto the next
// line, long flag or short. A matcher that saw only the one shape this tree
// happens to use would report PASS over the first page that wrote another, and
// under-recognition is the one way a census must not break.
//
// The last test is the other direction, and it is the one a rename gets right
// by accident: a claim label nothing tells anyone to look for is a lock nobody
// takes.

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

// labelFlag matches a label handed to `gh` by its long flag, with the value as
// written: double-quoted, single-quoted, or bare. `\s` admits a newline,
// because prose of this width wraps inside a fenced span and a flag parted from
// its value by a line break is still the instruction a session pastes.
var labelFlag = regexp.MustCompile("--(?:add-|remove-)?label[=\\s]\\s*(\"[^\"]*\"|'[^']*'|[^\\s\"'`]+)")

// labelShortFlag is `gh`'s one-letter spelling of the same flag. It is read
// only from the part of a line that FOLLOWS `gh `, because `git grep -l`,
// `wc -l` and `xargs -l` spell those two characters and mean something else.
var labelShortFlag = regexp.MustCompile("(?:^|\\s)-l[=\\s]\\s*(\"[^\"]*\"|'[^']*'|[^\\s\"'`]+)")

// labelSearchTerm matches the `label:` term of a search string, negated or not.
// Nothing may stand between the colon and the value, which is what tells a
// query from a YAML or TSX field: `label: "nav.contacts"` has the space that
// `label:"area: deals"` does not.
var labelSearchTerm = regexp.MustCompile("\\blabel:(\"[^\"]*\"|'[^']*'|[^\\s\"'`]+)")

// labelName is the shape of a name, and it is what keeps a flag's NEIGHBOUR out
// of the corpus: `--label --limit 10` and `--label $LABEL` name nothing. It also
// drops the `<x>` a page teaches the flag with, because a placeholder carries
// the one character a label may not.
var labelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _-]*(?:: [A-Za-z0-9][A-Za-z0-9 _-]*)?$`)

// trailingProse is what a bare value collects from the sentence or the fenced
// span it sits in: “-l bug` “ and `--add-label bug.` both name `bug`.
var trailingProse = regexp.MustCompile("[.,;:)\\]]+$")

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

func TestEveryLabelTheDocsNameIsDeclared(t *testing.T) {
	t.Parallel()

	declared := declaredLabels(t)
	named := labelsTheDocsName(t)

	if len(named) == 0 {
		t.Fatal("no page names a label this gate could read, so it measures nothing — " +
			"either the claim mechanism was removed and this file should go with it, " +
			"or the pages stopped spelling their labels in a form a session can run")
	}
	for label, pages := range named {
		if !slices.Contains(declared, label) {
			t.Errorf("%s tells a session to use %q, which .github/labels.yml does not declare.\n"+
				"\tThe command still exits 0 and returns nothing, which reads as "+
				"'nobody is on this' — so every session starts the same work. "+
				"Add the label to the source, or fix the spelling here.",
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
// direction: a label declared and searched for by nobody is a lock nobody
// takes. The source is the corpus, so a second claim label added tomorrow is
// covered without this file learning its name.
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
	named := labelsTheDocsName(t)
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

// TestTheMatcherReadsEveryShapeALabelIsHandedOverIn is the planted case for the
// census above, which can only fail SHORT: a shape it cannot see is a label
// nobody checks, reported as a clean tree. The tree spells none of these today,
// so reading the tree proves nothing about them and the shapes are written here.
func TestTheMatcherReadsEveryShapeALabelIsHandedOverIn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		text string
		want []string
	}{
		{"double-quoted", `gh issue edit 5 --add-label "status: in progress"`, []string{"status: in progress"}},
		{"single-quoted", `gh issue edit 5 --add-label 'status: in progress'`, []string{"status: in progress"}},
		{"equals sign", `gh issue list --label="area: deals"`, []string{"area: deals"}},
		{"bare unprefixed", `gh issue create --label bug`, []string{"bug"}},
		{"quoted unprefixed", `gh issue create --label "fast-track-debt"`, []string{"fast-track-debt"}},
		{"short flag", `gh issue create -l "status: in progress"`, []string{"status: in progress"}},
		{"short flag bare", "`gh issue create -l bug`", []string{"bug"}},
		{"remove flag", `gh issue edit 5 --remove-label "status: in progress"`, []string{"status: in progress"}},
		{"comma list", `gh issue edit 5 --add-label "bug,area: deals"`, []string{"area: deals", "bug"}},
		{"bare comma list", `gh issue create --label bug,enhancement`, []string{"bug", "enhancement"}},
		{"wrapped onto the next line", "a command spelled `--add-label\n\"status: in progress\"` in prose",
			[]string{"status: in progress"}},
		{"negated search term", `--search 'no:assignee -label:"status: in progress"'`, []string{"status: in progress"}},
		{"bare search term", `gh issue list --search "label:bug"`, []string{"bug"}},
		{"sentence punctuation", `Run gh issue edit 5 --add-label bug.`, []string{"bug"}},

		{"placeholder is not a name", `gh issue list --label "area: <x>"`, nil},
		{"a following flag is not a value", `gh issue list --label --limit 10`, nil},
		{"a shell variable is not a name", `gh issue edit 5 --add-label $LABEL`, nil},
		{"grep's own short flag", `git grep -l "claim: main-red"`, nil},
		{"line count", `wc -l README.md`, nil},
		{"xargs", `find . -name '*.md' | xargs -l echo`, nil},
		{"a short flag before the gh call", "`ls -l` first, then `gh pr list`", nil},
		{"a YAML or TSX field", `label: "nav.contacts"`, nil},
		{"a field holding an object", `label: { en: 'Contacts' }`, nil},
		{"prose ending on the word", `that is not a label: it is a field`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := labelsIn(tc.text); !slices.Equal(got, tc.want) {
				t.Errorf("labelsIn(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// labelsTheDocsName maps each label the tree's Markdown puts in front of a
// session to the pages naming it. Both directions read it: one asks whether
// every name is declared, the other whether every declaration is named.
//
// Held by: TestEveryLabelTheDocsNameIsDeclared,
// TestEveryDeclaredClaimLabelIsOneSomethingTellsSessionsToLookFor
func labelsTheDocsName(t *testing.T) map[string][]string {
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
		spelled := labelsIn(string(body))
		for _, match := range claimSpan.FindAllStringSubmatch(string(body), -1) {
			spelled = append(spelled, match[1])
		}
		for _, label := range spelled {
			if !slices.Contains(named[label], file.path) {
				named[label] = append(named[label], file.path)
			}
		}
	}
	return named
}

// labelsIn reads every label a text hands to `gh`, sorted and deduplicated.
//
// Held by: TestTheMatcherReadsEveryShapeALabelIsHandedOverIn
func labelsIn(text string) []string {
	found := labelValues(labelFlag, text)
	found = append(found, labelValues(labelSearchTerm, text)...)
	// The short flag is the one that needs a neighbour to be legible, so it is
	// read per line and only past the `gh ` that makes those two characters a
	// label flag rather than `ls -l`.
	for _, line := range strings.Split(text, "\n") {
		if command := strings.Index(line, "gh "); command >= 0 {
			found = append(found, labelValues(labelShortFlag, line[command:])...)
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// labelValues turns one pattern's captures into names: quotes off, a comma list
// split into its members, and anything not shaped like a name dropped.
//
// Held by: TestTheMatcherReadsEveryShapeALabelIsHandedOverIn
func labelValues(pattern *regexp.Regexp, text string) []string {
	var names []string
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		value := match[1]
		if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, "'") {
			value = value[1 : len(value)-1]
		}
		for _, member := range strings.Split(value, ",") {
			member = trailingProse.ReplaceAllString(strings.TrimSpace(member), "")
			if labelName.MatchString(member) {
				names = append(names, member)
			}
		}
	}
	return names
}
