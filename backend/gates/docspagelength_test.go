// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind budget H3

//go:build !integration

package gates

// A docs page an agent cannot navigate is a page it does not read, and the
// failure is silent: it greps, lands in the middle of six hundred lines, and
// answers from the part it happened to see. Length is the thing that makes that
// likely, so it gets a budget.
//
// This is a RATCHET, not a sweep. Ten of the tree's hand-written pages are over
// budget, and rewriting them in one change would be a diff nobody could review —
// so scripts/docs-page-length-waivers.txt freezes what was already here, and the
// rule is the one this repo already applies to Go: a page you TOUCH comes under
// budget.
//
// The waiver list only SHRINKS, and that is held by pinning its size below rather
// than asserted in prose. The first version of this file said the list was "closed
// to new entries" while nothing checked it: adding an over-budget page and its
// waiver line in the same change passed both directions. A comment claiming what
// no test holds is the defect *Reuse before you build* rule 4 names, and a gate is
// a bad place to commit it.
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

// docsPageCeiling returns the budget for a page, by the directory that owns it.
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
func docsPageCeiling(rel string) int {
	switch {
	case strings.HasPrefix(rel, "docs/how-to/"), strings.HasPrefix(rel, "docs/tutorials/"):
		return 350
	// EXPLANATION is the section that carries a mechanism, and it was measured
	// again in August 2026 because the number had stopped describing it. The
	// section holds 28 pages. Their 75th percentile is 404 by nearest rank and
	// 412 interpolated, so 1.5x is 606 to 618 — while the constant said 450,
	// which is about 1.1x. Four pages were over, all four waived, and a new
	// mechanism page was arriving over budget on its first commit.
	//
	// 620 is the top of that band, rounded. Deliberately NOT a number chosen to
	// clear the four waived pages: the longest is 519, so the band would have
	// freed them at any point in it. Picking the top rather than the middle is
	// the one judgement here, and it is made because 606 and 618 are the same
	// measurement read two defensible ways — a budget should not sit between
	// them and depend on which.
	case strings.HasPrefix(rel, "docs/explanation/"):
		return 620
	// A user guide is a narrative followed once, so it runs longer than a how-to
	// that is consulted a field at a time — the same reason an explanation does.
	case strings.HasPrefix(rel, "user-guide/"):
		return 450
	case strings.HasPrefix(rel, "docs/principles/"):
		return 250
	default:
		// Anything not named above — today reference/, evidence/ and the docs/
		// root, but written as "the rest" so a new section gets a budget the day
		// it appears instead of falling through a list that stopped describing
		// the tree.
		return 400
	}
}

// waivedPageBudget is how many pages the waiver list may name. It goes DOWN as
// pages come under budget, and a raise has to be written here — where a reviewer
// sees it in the diff and can ask why a new page needs exempting.
//
// What it does NOT catch, because nothing in this test can: a SWAP. Bring one
// waived page under budget, delete its line, and add a new over-budget page in the
// same change, and the count is unchanged and every check here is green. Detecting
// that needs a baseline of the list, and the list IS the baseline — the only place
// the addition is visible is the diff, which is where a reviewer reads it. Named
// here so the pin is not mistaken for a complete guard.
// 10 → 6 when the explanation budget was re-measured in August 2026: four
// waived pages were already inside the new number and left the list without a
// word of their prose changing. Lowering the pin in the same change is what
// keeps that a ratchet tooth rather than four spare slots — the swap above is
// still open, and a re-measure that freed pages without lowering the pin would
// widen it.
const waivedPageBudget = 6

const (
	docsWaiverFile = "../scripts/docs-page-length-waivers.txt"
	// The suite runs with backend/ as its working directory — the same relative
	// convention every sibling gate in this package uses.
	docsTreeRoot = ".."
)

// generatedDocMarker is what a generated page says about itself in its opening
// lines. The check reads that rather than keeping its own list of which pages are
// generated, because a list would go quietly short the day another generator
// landed.
//
// Deliberately NOT claiming every generator writes it: nothing here holds that,
// and it does not need to. A generator using a different marker would have its
// page budgeted and would fail this gate loudly — the safe direction — rather
// than slipping past it.
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
		line := strings.ToLower(strings.TrimSpace(scanner.Text()))
		// A COMPLETE HTML comment, with the marker inside it. Requiring only the
		// opener let a line that merely starts `<!--` and mentions the phrase
		// exempt a hand-written page; requiring only the phrase let any prose
		// about generated pages do the same. What is checked is the shape a
		// generator's header actually has.
		open := strings.Index(line, "<!--")
		if open < 0 {
			continue
		}
		closed := strings.Index(line[open:], "-->")
		if closed < 0 {
			continue
		}
		if strings.Contains(line[open:open+closed], generatedDocMarker) {
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

// documentedTrees are the prose trees this budget covers.
//
// user-guide/ is here and not only docs/ because otherwise "move it out of docs/"
// is a way PAST the budget rather than a change of audience — and this repository
// has just moved two pages that way for a good reason. A tree of prose is in scope
// wherever it sits.
var documentedTrees = []string{"docs", "user-guide"}

// handWrittenDocsPages walks the documented trees for markdown, derived from the
// tree so a new page is covered the moment it lands.
func handWrittenDocsPages(t *testing.T) []string {
	t.Helper()
	// TRACKED pages, not whatever the working tree carries. A page somebody is
	// drafting, or one a parallel session left behind, is not documentation
	// yet — and failing the build on it makes every local run untrustworthy,
	// which is worse than the budget going unenforced for one uncommitted
	// file. CI reads the committed tree, so a page over budget is caught at
	// the moment it becomes one of these pages.
	// ONE filter, in the walk. It has to be there — a page read before it is
	// filtered can stop the build on its contents — and a second copy here
	// would be a filter that cannot disagree with the first only for as long
	// as nobody edits either.
	tracked := gitTracked(t, docsTreeRoot)
	var pages []string
	for _, tree := range documentedTrees {
		pages = append(pages, handWrittenPagesUnder(t, tracked, tree)...)
	}
	sort.Strings(pages)
	return pages
}

func handWrittenPagesUnder(t *testing.T, tracked map[string]bool, tree string) []string {
	t.Helper()
	return handWrittenPagesRootedAt(t, docsTreeRoot, tracked, tree)
}

// handWrittenPagesRootedAt is the walk itself, over any root.
//
// The root is a parameter so a test can drive this over a repository built for
// the purpose. The branch that matters most here — the one that keeps an
// untracked draft from being READ — cannot be exercised against the committed
// tree, because the committed tree holds no untracked files: the real-tree gate
// passes for that reason rather than because the branch works, and an edit
// moving the filter back after isGeneratedDoc would go green.
func handWrittenPagesRootedAt(t *testing.T, treeRoot string, tracked map[string]bool, tree string) []string {
	t.Helper()
	var pages []string
	docsTreeRoot := treeRoot
	root := filepath.Join(docsTreeRoot, tree)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// NOTHING UNTRACKED IS READ AT ALL, which is a step earlier than the
		// tracked filter below and has to be.
		//
		// A file this gate reads before filtering is a file whose contents can
		// stop the build: isGeneratedDoc scans the page, and a draft with one
		// very long line fatals the scanner. Filtering afterwards leaves every
		// local run hostage to what a working tree happens to hold, which is
		// the complaint this change started from.
		//
		// Directories are still descended, because a tracked page can live
		// under an untracked directory only if git tracks it — and then git
		// names it here. The symlink refusal below is about the REPOSITORY: a
		// link somebody committed hides a subtree from the budget, and one
		// nobody committed hides nothing.
		rel, relErr := filepath.Rel(docsTreeRoot, path)
		if relErr != nil {
			return relErr
		}
		if !d.IsDir() && !tracked[filepath.ToSlash(rel)] {
			return nil
		}
		// filepath.WalkDir does not follow symlinks, so a symlinked directory
		// under docs/ would be skipped and its pages never counted — the gate
		// would read a smaller tree and report the same word for it, PASS. That is
		// the one way a census must not break, so refuse rather than walk past it.
		// Nothing under docs/ is a link today; this exists so that staying true is
		// checked rather than assumed.
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink: this walk does not follow them, so its target "+
				"would go uncounted and the budget would silently cover a smaller tree. "+
				"Replace it with the real file or directory", path)
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		if isGeneratedDoc(t, path) {
			return nil
		}
		pages = append(pages, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return pages
}

func docsPageLineCount(t *testing.T, rel string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsTreeRoot, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return len(strings.Split(strings.TrimRight(string(raw), "\n"), "\n"))
}

func TestEveryHandWrittenDocsPageFitsItsBudget(t *testing.T) {
	t.Parallel()
	pages := handWrittenDocsPages(t)
	if len(pages) == 0 {
		t.Fatal("found no hand-written page in any documented tree — this gate would pass by " +
			"having nothing to check, which is the one way it must not break")
	}
	waived := readDocsWaivers(t)

	for _, rel := range pages {
		ceiling, got := docsPageCeiling(rel), docsPageLineCount(t, rel)
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

	if len(waived) > waivedPageBudget {
		t.Errorf("%s names %d pages, over the pinned budget of %d.\n"+
			"The list only shrinks. If a page genuinely has to be exempted, lower a different "+
			"entry in the same change or raise waivedPageBudget and say why in the pull request — "+
			"the point is that it cannot happen silently.",
			filepath.Base(docsWaiverFile), len(waived), waivedPageBudget)
	}
	if slack := waivedPageBudget - len(waived); slack > 2 {
		t.Errorf("%s names %d pages against a pinned budget of %d. Lower the pin: a ratchet "+
			"with %d spare entries stops noticing the growth it exists to catch.",
			filepath.Base(docsWaiverFile), len(waived), waivedPageBudget, slack)
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
	t.Parallel()
	dir := t.TempDir()
	cases := map[string]struct {
		body string
		want bool
	}{
		"generated.md":   {"# Title\n\n<!-- Generated together with x.json; do not edit by hand. -->\n", true},
		"prose.md":       {"# Title\n\nThis page explains how the generated Go is produced.\n", false},
		"late-marker.md": {"# T\n\na\n\nb\n\nc\n\nd\n\n<!-- do not edit by hand -->\n", false},
		// The phrase in ordinary prose is not a marker. Without this the page
		// exempts itself from the budget just by mentioning the rule.
		"phrase-in-prose.md": {"# T\n\nThese pages are generated, so do not edit by hand.\n", false},
		// An opener with no terminator is not a generator's header; neither is a
		// sentence that merely opens with one.
		"unterminated.md":       {"# T\n\n<!-- do not edit by hand\n", false},
		"marker-after-close.md": {"# T\n\n<!-- note --> do not edit by hand\n", false},
		"generated-cap.md":      {"# T\n\n<!-- GENERATED by make x — DO NOT EDIT BY HAND. -->\n", true},
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

// AN UNTRACKED PAGE IS NOT READ, which is a step earlier than not being judged.
//
// The distinction is the whole complaint this file answers: a page the walk
// READS can stop the build on its contents, whatever the budget then decides
// about it. isGeneratedDoc scans the first lines, and a draft with one line past
// the scanner's buffer fatals it.
//
// Driven over a repository built for the purpose. The committed tree holds no
// untracked files, so the real-tree gate passes whether this branch works or
// not — an edit moving the filter back after isGeneratedDoc would break nothing
// there and be found by whoever next ran an install locally.
//
// Here it is found immediately, and LOUDLY: the draft's line is past the
// scanner's buffer, so a walk that reads it fatals rather than merely counting
// one page too many. That is the same failure the reports described, reproduced
// on demand instead of waited for.
func TestAnUntrackedDraftIsNeitherReadNorJudged(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, write := initRepo(t, root, map[string]string{"how-to/committed.md": "# committed\n"})
	// The draft: present, not committed, and carrying a line no scanner will
	// read. Over the budget as well, so it would fail on length even if it
	// survived being read.
	write("how-to/a-draft.md", strings.Repeat("x", 2<<20)+"\n")

	pages := handWrittenPagesRootedAt(t, root, gitTracked(t, root), "how-to")
	if len(pages) != 1 || pages[0] != "how-to/committed.md" {
		t.Fatalf("pages = %v, want only the committed one — a draft nobody committed is not "+
			"documentation yet, and reading it at all is what lets its contents stop the build", pages)
	}
}
