// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

//go:build !integration

package gates

// Every page under docs/ is reachable by following links from docs/README.md.
//
// The sibling gate (docslinktargets_test.go) holds the other direction: a link
// that names no file. This one holds the failure with no symptom at all — a page
// nobody links. It renders correctly, it passes every other gate, and the only
// evidence that it exists is somebody running `find`. Twenty-seven pages had
// reached that state at once, and the tree they hid included the whole German
// compliance pack, which is the paperwork a customer signs before an installation
// may read employee mail. Nothing was broken; it was simply unfindable.
//
// The walk is transitive on purpose, so a subtree may keep its own hub page —
// docs/handbook/README.md and docs/compliance/en/README.md both index their own
// trees, and a flat "is this basename mentioned in the index" check would either
// force every page into the root index or accept a filename that appears in some
// unrelated sentence. Reachability is the property that actually matters to a
// reader, and it is the one derived from the tree rather than from a list here.
//
// There is no skip-list. A page that should not be advertised is still a page a
// reader can arrive at, so the answer is a line in the index saying what it is —
// which is what docs/evidence/ got — and not an exemption here, because an
// exemption is the second place a census learns to under-report.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docsIndex is the one page every other page must be reachable from.
const docsIndex = "README.md"

// linkedPages returns the docs-relative .md targets one page links to.
//
// It reads links through the sibling gate's relativeLinkTargets rather than
// spelling a second markdown grammar. Two readers of one syntax drift, and this
// gate drifts in the silent direction: a link the OTHER gate strips as an
// EXAMPLE — the ones inside fenced blocks on the pages that teach the link rule —
// is a link a reader cannot click, so counting it here would mark an orphan page
// reachable and report the clean tree it never checked. That is the same census
// failing short that this gate exists to catch, one level up.
//
// The reference-style caveat named on relativeLinkTargets applies here too, and
// costs more: an unresolved `[text][label]` is a link this walk does not follow,
// so a page reachable only that way reports as an orphan. That direction is
// loud — somebody reads the failure and adds an inline link — which is why it is
// acceptable while the tree uses no such link for an in-tree path.
func linkedPages(t *testing.T, docsRoot, page string) []string {
	t.Helper()
	// Lstat, not the plain read: os.ReadFile FOLLOWS a symlink, so a tracked
	// `docs/x.md -> ../../outside` reachable from the index would be read and
	// mined for link targets — marking real orphans reachable, and in a public
	// repository printing whatever it found. migrationcitations refuses one for
	// the same reason.
	full := filepath.Join(docsRoot, page)
	info, err := os.Lstat(full)
	if err != nil {
		t.Fatalf("reading %s: %v", page, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("reading %s: %v", page, err)
	}

	var out []string
	for _, link := range relativeLinkTargets(string(raw)) {
		if !strings.HasSuffix(link.target, ".md") {
			continue
		}
		// Resolved against the linking page's own directory, the way a reader's
		// markdown viewer resolves it. A link that climbs out of docs/ is a link
		// to the repository root, which this gate does not own.
		resolved := filepath.Clean(filepath.Join(filepath.Dir(page), link.target))
		if strings.HasPrefix(resolved, "..") {
			continue
		}
		out = append(out, resolved)
	}
	return out
}

// docsPages returns every markdown page under docs/, docs-relative.
//
// TRACKED files only, and symlinks refused — both for reasons the siblings in
// this package already paid for. filepath.WalkDir does not descend a symlinked
// directory, so a committed `docs/handbook -> …` would make that whole subtree
// invisible and this gate would report PASS over a smaller tree; docspagelength
// refuses one on exactly that reasoning. And walking the working tree rather
// than the index makes every local run hostage to whatever scratch page happens
// to sit under docs/, which reds the gate on a file nobody committed.
func docsPages(t *testing.T) []string {
	t.Helper()
	var pages []string
	for _, f := range trackedFiles(t) {
		if f.symlink || !strings.HasPrefix(f.path, "docs/") || !strings.HasSuffix(f.path, ".md") {
			continue
		}
		pages = append(pages, strings.TrimPrefix(f.path, "docs/"))
	}
	if len(pages) == 0 {
		t.Fatal("no markdown found under docs/ — this gate is reading a tree shape that is gone")
	}
	return pages
}

func TestEveryDocsPageIsReachableFromTheIndex(t *testing.T) {
	t.Parallel()
	const docsRoot = "../docs"

	seen := map[string]bool{docsIndex: true}
	queue := []string{docsIndex}
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		for _, next := range linkedPages(t, docsRoot, page) {
			if seen[next] {
				continue
			}
			// A link naming a file that does not exist is the sibling gate's
			// finding, not this one's. Following it here would fail this test
			// with the wrong sentence.
			if _, err := os.Stat(filepath.Join(docsRoot, next)); err != nil {
				continue
			}
			seen[next] = true
			queue = append(queue, next)
		}
	}

	for _, page := range docsPages(t) {
		if !seen[page] {
			t.Errorf("docs/%s is reachable from no page under docs/README.md — a reader can only find it with `find`. Link it from the index, or from the hub page for its tree", page)
		}
	}
}
