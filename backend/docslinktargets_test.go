// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

//go:build !integration

package backendarch

// Every relative link under docs/ points at a file that exists.
//
// docs/ is where the rulebook sends a reader for the reasoning behind a rule, so
// a link that does not resolve is not cosmetic — it is the reader arriving
// nowhere at the moment they were told to go and read. Markdown has no build
// step: a broken relative link renders as a normal link and fails only when
// somebody clicks it, which is why four of them sat in this tree at once. Three
// were the same mistake — a repo-root path (`docs/reference/modules.md`,
// `frontend/src/design-system/README.md`, `SECURITY.md`) written into a page that
// is not at the repo root, so it resolved against the page's own directory.
//
// This is deliberately a resolution check and not a link checker: an http(s)
// target is somebody else's uptime, and an anchor is checked by nothing here (a
// missing one scrolls to the top, which the direction gate in
// rulebookdirection_test.go addresses by forbidding the upward links that carried
// nearly all of them). What is left is the part that is decidable from the tree
// alone, and it is the part that broke.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// markdownLink captures the target of an inline markdown link, stopping at the
// first whitespace so an optional `"title"` is not swallowed into the path.
var markdownLink = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)`)

// codeSpan matches a backticked span, and fence matches a fenced-block delimiter.
//
// Both must be stripped before the link pattern runs, because Go and TypeScript
// generics put a `]` immediately before a `(`: `From[K](u)` in
// docs/reference/platform-toolkit.md is a documented function signature that
// reads as a link to a file named `u`. Prose is the corpus; code is not.
var (
	codeSpan = regexp.MustCompile("`[^`]*`")
	fence    = regexp.MustCompile("^\\s*```")
)

func TestEveryRelativeLinkUnderDocsResolves(t *testing.T) {
	const docsRoot = "../docs"

	scanned := 0
	err := filepath.WalkDir(docsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		for _, link := range relativeLinkTargets(string(raw)) {
			resolved := filepath.Join(filepath.Dir(path), link.target)
			if _, statErr := os.Stat(resolved); statErr != nil {
				t.Errorf("%s:%d links to %q, which does not exist (resolved to %s)\n"+
					"A relative link resolves against the directory of the page it is written in, "+
					"not the repository root — a repo-root path such as `docs/reference/modules.md` "+
					"pasted into docs/explanation/ becomes docs/explanation/docs/reference/. "+
					"Markdown has no build step, so this fails only when a reader clicks it. "+
					"Write the path relative to this file, or name the target in prose instead.",
					path, link.line, link.target, resolved)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", docsRoot, err)
	}
	if scanned == 0 {
		t.Fatalf("no markdown found under %s — this gate is reading a tree shape that is gone", docsRoot)
	}
}

// docsLink is one in-tree link target and the line it was written on.
type docsLink struct {
	line   int
	target string
}

// eachProseLine calls fn with the 1-based number and the code-free text of every
// PROSE line in a markdown document: fenced blocks are skipped and inline code
// spans are blanked out.
//
// Both link gates need exactly this. A page that teaches a link rule wants to
// SHOW the link it forbids, and a matcher reading raw markdown fails the page for
// its own example — so each gate has to strip first, and two spellings of the
// stripping is the shape *Reuse before you build* is about. One writer, here.
func eachProseLine(doc string, fn func(lineNo int, text string)) {
	inFence := false
	for i, line := range strings.Split(doc, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		fn(i+1, codeSpan.ReplaceAllString(line, ""))
	}
}

// relativeLinkTargets returns the in-tree link targets of one markdown document,
// with fenced blocks and inline code removed and anchors trimmed.
//
// INLINE links only — `[text](target)`. A reference-style `[text][label]` with a
// `[label]: target` definition is not resolved, so a broken one of those would go
// unseen: this census can fail short in exactly that shape. Nothing under docs/
// uses the form for an in-tree path today (the two definitions that exist are
// external URLs, which this gate skips anyway), and resolving labels means
// carrying a second markdown parser. Named here rather than left to be discovered
// as a silent PASS.
func relativeLinkTargets(doc string) []docsLink {
	var links []docsLink
	eachProseLine(doc, func(lineNo int, text string) {
		for _, m := range markdownLink.FindAllStringSubmatch(text, -1) {
			if target := inTreeTarget(m[1]); target != "" {
				links = append(links, docsLink{line: lineNo, target: target})
			}
		}
	})
	return links
}

// inTreeTarget reduces one raw link target to a path this tree can check, or ""
// when the target is not one: an absolute URL, a mail or telephone scheme, or a
// pure same-page anchor.
func inTreeTarget(raw string) string {
	if strings.Contains(raw, "://") || strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "tel:") {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
}
