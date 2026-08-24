// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package backendarch

// A docs page an agent cannot navigate is a page it does not read, and the
// failure is silent: it greps, lands in the middle of six hundred lines, and
// answers from the part it happened to see. Length is the thing that makes that
// likely, so it gets a budget.
//
// This is a RATCHET, not a sweep. Ten of the tree's hand-written pages are over
// budget, and rewriting them in one change would be a diff nobody could
// review — so scripts/docs-page-length-waivers.txt freezes what was
// already here, and the rule is the one this repo already applies to Go: a page
// you TOUCH comes under budget. The waiver file is CLOSED to new entries and only
// shrinks.
//
// Two things it deliberately does not do. It does not judge a GENERATED page:
// those are machine output that no author can shorten, and they are recognised by
// the header they carry rather than by a list kept here — a list would be the
// second copy of the generated set that *Reuse before you build* rule 5 is about,
// and it would go quietly short the day another generator landed. And it does not
// set one number for the whole tree: a how-to that cannot be followed in one
// screen has failed at something a reference table has not.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ceilingFor returns the budget for a page, by the directory that owns it.
//
// These numbers are MEASURED, not chosen. Each is roughly 1.5x its section's
// 75th percentile today, which makes the gate an outlier detector rather than a
// demand that every page be short: a how-to is a procedure and is tightest, an
// explanation carries a mechanism and earns more, a reference is a table bounded
// by what it tabulates.
//
// The first draft of this gate used 150 and 250, picked before measuring. That
// put 45 of the tree's pages over budget — a waiver file nobody would ever work
// through, which is a waiver that has rotted on arrival and reads to the next
// author as a list of pages allowed to be long. At these numbers it is 10, and
// six of those are within a quarter of their budget.
func ceilingFor(rel string) int {
	switch {
	case strings.HasPrefix(rel, "docs/how-to/"), strings.HasPrefix(rel, "docs/tutorials/"):
		return 350
	case strings.HasPrefix(rel, "docs/explanation/"):
		return 450
	case strings.HasPrefix(rel, "docs/principles/"):
		return 250
	default:
		// reference/, evidence/, and the two pages at the docs/ root.
		return 400
	}
}

const (
	docsWaiverFile = "../scripts/docs-page-length-waivers.txt"
	// The suite runs with backend/ as its working directory — the same relative
	// convention every sibling gate in this package uses.
	docsTreeRoot = ".."
)

// generatedDocMarker is what a generated page says about itself in its opening
// lines. Every generator in this tree writes one, and the check reads it rather
// than keeping its own list of which pages are generated.
const generatedDocMarker = "do not edit by hand"

func isGeneratedDoc(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			t.Errorf("close %s: %v", path, cerr)
		}
	}()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for i := 0; scanner.Scan() && i < 5; i++ {
		if strings.Contains(strings.ToLower(scanner.Text()), generatedDocMarker) {
			return true
		}
	}
	// A scan error is not "not generated" — it would exempt nothing and waive
	// nothing, but it would also mean this page was never really read.
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return false
}

func readDocsWaivers(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(docsWaiverFile)
	if err != nil {
		t.Fatalf("read %s: %v", docsWaiverFile, err)
	}
	waived := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		waived[line] = true
	}
	return waived
}

// handWrittenDocsPages walks docs/ for markdown, derived from the tree so a new
// page is covered the moment it lands.
func handWrittenDocsPages(t *testing.T) []string {
	t.Helper()
	var pages []string
	root := filepath.Join(docsTreeRoot, "docs")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		if isGeneratedDoc(t, path) {
			return nil
		}
		rel, relErr := filepath.Rel(docsTreeRoot, path)
		if relErr != nil {
			return relErr
		}
		pages = append(pages, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	sort.Strings(pages)
	return pages
}

func lineCount(t *testing.T, rel string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsTreeRoot, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return len(strings.Split(strings.TrimRight(string(raw), "\n"), "\n"))
}

func TestEveryHandWrittenDocsPageFitsItsBudget(t *testing.T) {
	pages := handWrittenDocsPages(t)
	if len(pages) == 0 {
		t.Fatal("found no hand-written page under docs/ — this gate would pass by having " +
			"nothing to check, which is the one way it must not break")
	}
	waived := readDocsWaivers(t)

	for _, rel := range pages {
		ceiling, got := ceilingFor(rel), lineCount(t, rel)
		switch {
		case got <= ceiling && waived[rel]:
			// The ratchet's teeth. A page that came under budget must leave the
			// waiver file, or the file stops describing anything and the next
			// author reads it as a list of pages that are allowed to be long.
			t.Errorf("%s is %d lines, inside its budget of %d, but still sits in %s.\n"+
				"Delete its line: the waiver file records what was over budget when this gate "+
				"landed, and it only shrinks.", rel, got, ceiling, filepath.Base(docsWaiverFile))
		case got > ceiling && !waived[rel]:
			t.Errorf("%s is %d lines, over the %d-line budget for its section.\n"+
				"Cut it, or split it. A page an agent cannot hold at once is one it answers from "+
				"the middle of. The cross-reference preamble most pages open with is usually the "+
				"cheapest ten lines to lose.\n"+
				"This budget is not waivable for a new page: %s is closed to new entries.",
				rel, got, ceiling, filepath.Base(docsWaiverFile))
		}
	}

	// A waiver naming a page that no longer exists is a line nobody will ever
	// remove on purpose.
	present := map[string]bool{}
	for _, rel := range pages {
		present[rel] = true
	}
	for rel := range waived {
		if !present[rel] {
			t.Errorf("%s waives %s, which is not a hand-written page under docs/ any more — "+
				"delete the line", filepath.Base(docsWaiverFile), rel)
		}
	}
}

// TestTheDocsBudgetGateSeesAGeneratedPageAsGenerated plants the case the exemption
// is about, so the exemption cannot quietly widen into "any page with a comment
// near the top".
func TestTheDocsBudgetGateSeesAGeneratedPageAsGenerated(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]struct {
		body string
		want bool
	}{
		"generated.md":     {"# Title\n\n<!-- Generated together with x.json; do not edit by hand. -->\n", true},
		"prose.md":         {"# Title\n\nThis page explains how the generated Go is produced.\n", false},
		"late-marker.md":   {"# T\n\na\n\nb\n\nc\n\nd\n\n<!-- do not edit by hand -->\n", false},
		"generated-cap.md": {"# T\n\n<!-- GENERATED by make x — DO NOT EDIT BY HAND. -->\n", true},
	}
	for name, tc := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if got := isGeneratedDoc(t, path); got != tc.want {
			t.Errorf("%s: generated=%v, want %v — %s", name, got, tc.want,
				fmt.Sprintf("the marker is read from the first five lines, case-insensitively"))
		}
	}
}
