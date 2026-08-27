// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The label taxonomy is written down once and read from there.
//
// It used to be written down three times: the long-form reference, the
// rulebook's digest of it, and the repository's own labels — the only one of
// the three that does anything. Nothing connected them, so adding an area,
// renaming one or retiring one meant remembering all three, and two of the
// three were prose. That fails SILENTLY in both directions: an agent reads a
// list naming a label that does not exist, files with it, and `gh issue create`
// fails; or reads one that omits a label that does exist and picks a
// worse-fitting area, which nothing ever notices.
//
// `.github/labels.yml` is the source now. This gate is the offline half of
// making it one — it asserts the reference page names exactly the labels the
// file declares, in both directions — and `scripts/sync-labels.sh` reconciles
// the repository from the same file, which is what stops the file being a
// fourth copy.
//
// The direction that matters is the one a hand-kept list gets wrong. A gate
// asserting only "every documented label exists in the file" would pass a file
// that had grown a label the page never mentions, which is the omission an
// agent silently pays for. So both are checked, and each failure says which
// half moved.
//
// DELIBERATELY OFFLINE. The authoritative list lives on GitHub and `make check`
// must pass without a network, reproducibly — so this gate cannot ask GitHub
// anything, and does not try. What it proves is that the two copies IN THE TREE
// agree; the workflow is what makes the tree's copy true.
//
// The four taxonomy sections are judged and GitHub's own defaults are not.
// `documentation`, `good first issue`, `help wanted` and `dependencies` are what
// a drive-by contributor and Dependabot reach for; they are labels this
// repository has rather than a taxonomy it teaches, and requiring the reference
// page to document them would put four rows in front of a reader that answer no
// question the page is for.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The two files, from the module root the gates package chdirs to.
var (
	labelSourcePath = filepath.Join(repoRoot, ".github", "labels.yml")
	labelDocPath    = filepath.Join(repoRoot, "docs", "reference", "issue-labels.md")
)

// declaredLabel matches one entry of the source file. A hand-rolled reader
// rather than a YAML dependency: the file's shape is fixed by the action that
// consumes it, three scalar fields in a fixed order, and a parser that accepted
// more than the action does would pass a file the action then rejects.
var declaredLabel = regexp.MustCompile(`(?m)^- name: "([^"]*)"\n  color: "([0-9a-fA-F]{6})"\n  description: "([^"]*)"$`)

// githubDefaults are the labels this repository has and does not teach.
var githubDefaults = []string{"documentation", "good first issue", "help wanted", "dependencies"}

// The page's own sections, and how each spells the labels it lists.
//
// Driven by the SECTION rather than by a pattern for "what a label name looks
// like", because a pattern is a hand-kept list of the taxonomy in a third
// spelling — the shape this gate exists to remove. A section, by contrast,
// derives its subjects: a provenance label added to the source and to that
// paragraph agrees, and one added to only one of them does not, without this
// file learning its name.
var labelSections = []struct {
	heading string
	// prefix the source spells these labels with; "" for the unprefixed axis.
	prefix string
	// bare is true where the page names them without that prefix, which is what
	// the Area run and the Provenance sentence do — fifteen names read as a set
	// where fifteen `area: x` would read as fifteen rows.
	bare bool
}{
	{heading: "## Priority", prefix: "priority: "},
	{heading: "## Area", prefix: "area: ", bare: true},
	{heading: "## Status", prefix: "status: "},
	{heading: "## Provenance", prefix: "", bare: true},
}

// labelSpan matches a label name as the page writes one: a fenced span holding
// only the name, so `gh issue list --label "area: <x>"` — which has spaces — is
// prose about a label rather than a listing of one.
var labelSpan = regexp.MustCompile("`((?:priority: |area: |status: )?[a-z][a-z-]*)`")

func TestEachSectionOfTheReferencePageListsExactlyItsLabels(t *testing.T) {
	t.Parallel()

	declared := declaredLabels(t)
	page := readLabelDoc(t)

	for _, section := range labelSections {
		t.Run(strings.TrimPrefix(section.heading, "## "), func(t *testing.T) {
			want := labelsOfAxis(declared, section.prefix)
			if len(want) == 0 {
				t.Fatalf("the source declares no label for %s, so this case measures nothing — "+
					"an axis with no members is a section the page should not have either",
					section.heading)
			}
			got := listedIn(page, section.heading, section.prefix, section.bare)
			if !slices.Equal(want, got) {
				t.Errorf("%s lists %v and the source declares %v.\n"+
					"\tThis section is what a person reads when choosing one, so a name only in "+
					"the source is a label nobody picks, and a name only here is one "+
					"`gh issue create` will refuse.", section.heading, got, want)
			}
		})
	}
}

// labelsOfAxis reads one axis out of the source, sorted, with the GitHub
// defaults left out. An empty prefix means the unprefixed axis — provenance —
// which is whatever remains rather than a list written here.
//
// Held by: TestEachSectionOfTheReferencePageListsExactlyItsLabels
func labelsOfAxis(declared []string, prefix string) []string {
	var want []string
	for _, label := range declared {
		if slices.Contains(githubDefaults, label) {
			continue
		}
		switch {
		case prefix != "":
			if name, ok := strings.CutPrefix(label, prefix); ok {
				want = append(want, name)
			}
		case !strings.Contains(label, ": "):
			want = append(want, label)
		}
	}
	slices.Sort(want)
	return want
}

// listedIn reads the labels one section names, stripped to their bare form so
// both spellings compare against one set.
//
// Held by: TestEachSectionOfTheReferencePageListsExactlyItsLabels
func listedIn(page, heading, prefix string, bare bool) []string {
	section := page
	start := strings.Index(section, "\n"+heading+"\n")
	if start < 0 {
		return nil
	}
	section = section[start+len(heading)+2:]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	var listed []string
	for _, match := range labelSpan.FindAllStringSubmatch(section, -1) {
		// A section names its own axis and nothing else: a bare word in the
		// Priority section is prose, and a prefixed name in the Provenance
		// section is a cross-reference rather than a listing. `strings.CutPrefix`
		// is not the test — an empty prefix "matches" every string — so the
		// unprefixed axis is asked its own question: no axis prefix at all.
		name := match[1]
		if prefix == "" {
			if strings.Contains(name, ": ") {
				continue
			}
		} else if bare {
			if strings.Contains(name, ": ") {
				continue
			}
		} else {
			cut, ok := strings.CutPrefix(name, prefix)
			if !ok {
				continue
			}
			name = cut
		}
		if !slices.Contains(listed, name) {
			listed = append(listed, name)
		}
	}
	slices.Sort(listed)
	return listed
}

func declaredLabels(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(labelSourcePath)
	if err != nil {
		t.Fatalf("reading %s: %v", labelSourcePath, err)
	}
	var declared []string
	for _, match := range declaredLabel.FindAllStringSubmatch(string(source), -1) {
		declared = append(declared, match[1])
	}
	// A parse that found nothing would agree with a page that lists nothing,
	// which is the shape this whole gate exists to refuse.
	if len(declared) == 0 {
		t.Fatalf("%s declared no label this gate could read — either the file is empty or its "+
			"shape changed and this parser now matches nothing, and a parser matching nothing "+
			"agrees with every page", labelSourcePath)
	}
	return declared
}

func readLabelDoc(t *testing.T) string {
	t.Helper()
	page, err := os.ReadFile(labelDocPath)
	if err != nil {
		t.Fatalf("reading %s: %v", labelDocPath, err)
	}
	return string(page)
}
