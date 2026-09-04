// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

//go:build !integration

package gates

// Every source file a prose page names in backticks still exists.
//
// The docs are written to be followed, and the thing a reader follows first is a
// path: "the screen is `frontend/src/screens/inbox.tsx`". When that file is
// renamed or folded into another, the sentence keeps rendering and keeps reading
// as true. Four had rotted this way — an approvals screen that became a lane of
// the worklist, a connections card that became the relationship-graph card, and
// two components deleted outright — and each one sends a reader to a file that
// is not there, in the one document written to save them the search.
//
// SCOPE, and why it is drawn here. Only backticked spans that look like a path
// and carry a source extension are judged. A bare filename is resolved anywhere
// in the tree, because pages legitimately name `storekit.go` without a
// directory. A path WITH a separator resolves from the repository root, from
// backend/, or as a TAIL of a tracked path — pages write `screens/worklist.tsx`
// for a file three directories deeper, and rejecting that would fail correct
// prose. Prose that merely mentions a name is not a claim about a path, and this
// gate does not read it.
//
// WHAT IT CANNOT SEE. A path that resolves but has since changed MEANING — a
// file that still exists and no longer does what the sentence says — passes
// here, and no test can catch that. This holds the mechanical half: the claim
// that is decidable from the tree.
//
// Migration .sql files are the one kind this gate does not rule on: the register
// in danglingcitations.txt and its own gate already own that question, and a
// second opinion would be a second copy of that decision. Only the .sql files —
// the Go and testdata files under migrations/ are cited like any other code.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pathShaped matches a span that could be a path: no spaces, no punctuation a
// path does not carry. A span that fails this is prose in backticks.
var pathShaped = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

// codeSpanContents captures what an inline code span holds. eachProseLine BLANKS
// spans rather than yielding them, because its other reader wants the prose
// around them; this gate wants the spans themselves, so it re-reads the raw line.
var codeSpanContents = regexp.MustCompile("`([^`\n]+)`")

// sourceExtensions are the file kinds a page names when it is pointing a reader
// at code. Prose extensions are absent on purpose: a .md target is a LINK, and
// docslinktargets_test.go already resolves those with the anchor rules a link
// needs.
var sourceExtensions = []string{".go", ".ts", ".tsx", ".sql", ".yaml", ".yml", ".json", ".sh", ".mjs", ".css"}

// codeExtensions are the kinds this gate will chase from a BARE filename.
//
// The data formats are deliberately absent. A page writing `manifest.json` or
// `data.json` with no directory is usually naming a file inside something the
// product produces — an export bundle, a response body — not a file in this
// repository, and there is no way to tell those apart from the name alone. A
// bare `.go`/`.tsx` name has no such second reading: it is this tree or it is
// nothing. Written as the narrow set rather than as exceptions to the wide one,
// so a new data format does not silently join the chased kinds.
var codeExtensions = []string{".go", ".ts", ".tsx", ".sh", ".mjs", ".css"}

// bareMigrationFile matches a migration filename cited without a directory.
var bareMigrationFile = regexp.MustCompile(`^[0-9]{4,14}_[a-z0-9_]+\.(up|down)\.sql$`)

// notAPathClaim reports a span this gate must not read as a claim about a file.
//
// Three kinds, and each was learned from a false positive rather than guessed:
//
//   - A span that is only an EXTENSION. Pages write `.go`, `.tsx`, `.down.sql`
//     constantly to name a KIND of file, and reading those as paths produced
//     more noise than findings — which is how a gate gets weakened until it
//     holds nothing. Anything starting with "." is out, which also removes the
//     `.tmp/` and `.build/` scratch paths that exist only while a command runs.
//   - A SHAPE rather than a name: `<name>`, `NNNN_name.up.sql`, an ellipsis, a
//     relative `../` example. A how-to has to be able to write the form a reader
//     will fill in, and failing those would push authors to stop writing the
//     form at all.
//   - A migration .sql file. The dangling-citation register in
//     danglingcitations.txt and its own gate already rule on those, and a second
//     opinion here would be a second copy of that decision. Narrowed to .sql
//     deliberately: excluding everything under a migrations/ directory also made
//     `migrations/migrations_test.go` and `migrations/testdata/head_catalog.txt`
//     uncitable, which is a whole package this gate stopped reading for nothing.
//
// Everything left is a span that claims a file exists.
func notAPathClaim(p string) bool {
	switch {
	case strings.HasPrefix(p, "."),
		strings.ContainsAny(p, "<>*"),
		strings.Contains(p, "NNNN"),
		strings.Contains(p, ".."),
		strings.Contains(p, "migrations/") && strings.HasSuffix(p, ".sql"),
		bareMigrationFile.MatchString(p):
		return true
	}
	return false
}

// treeIndex indexes the tracked tree twice — by full relative path, and by every
// tail of each path — so both citation styles resolve against one reading.
type treeIndex struct {
	paths map[string]bool
	// suffixes holds every tail of every tracked path, so a page may name a file
	// relative to a directory this gate does not know — `screens/worklist.tsx`
	// for frontend/src/screens/worklist.tsx. The one-segment tails are the bare
	// basenames, so a separate index of those would be a subset of this one.
	suffixes map[string]bool
}

func indexTree(t *testing.T) treeIndex {
	t.Helper()
	idx := treeIndex{paths: map[string]bool{}, suffixes: map[string]bool{}}
	for _, f := range trackedFiles(t) {
		idx.paths[f.path] = true
		parts := strings.Split(f.path, "/")
		for i := range parts {
			idx.suffixes[strings.Join(parts[i:], "/")] = true
		}
	}
	if len(idx.paths) == 0 {
		t.Fatal("the index is empty — this gate is reading a tree shape that is gone")
	}
	return idx
}

// resolves reports whether the tree carries the file a page named.
//
// A tracked path is the ordinary answer. An IGNORED one counts too, and has to:
// `config/margince.yaml` is the operator's own deployment file and
// `build/composition/` is what `make composition` writes, so both are real files
// a reader will find and neither is in the index. Asking git rather than keeping
// a list of such directories means a new build output is covered the day the
// ignore rule for it lands.
func (idx treeIndex) resolves(cited string) bool {
	if idx.paths[cited] || idx.paths["backend/"+cited] || idx.suffixes[cited] {
		return true
	}
	if !strings.Contains(cited, "/") {
		for _, ext := range codeExtensions {
			if strings.HasSuffix(cited, ext) {
				return false
			}
		}
		// A bare data-format name: not a claim this gate can decide.
		return true
	}
	return idx.ignored(cited)
}

// ignored reports whether git would ignore the path, checked from the repository
// root. An error answers "not ignored": the gate must not be talked out of a
// finding by a command that failed to run.
func (idx treeIndex) ignored(cited string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", "--", cited)
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}

// citedSourcePaths returns the source paths one page names in backticks, with
// the line each was written on.
//
// Fenced blocks are skipped through the same `fence` pattern docslinktargets
// uses, because a fence is where a page SHOWS a form rather than claims a file,
// and judging inside one fails a how-to for its own example. It cannot go
// through that file's eachProseLine, which BLANKS code spans on its way out —
// the other reader wants the prose around them, this one wants the spans
// themselves, and feeding this gate blanked lines would leave it matching
// nothing while still reporting PASS.
func citedSourcePaths(doc string) []citedPath {
	var cited []citedPath
	seen := map[string]bool{}
	inFence := false
	for i, line := range strings.Split(doc, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range codeSpanContents.FindAllStringSubmatch(line, -1) {
			p, ok := sourcePathIn(m[1])
			if !ok || seen[p] {
				continue
			}
			seen[p] = true
			cited = append(cited, citedPath{line: i + 1, path: p})
		}
	}
	return cited
}

// citedPath is one source path a page named, and where.
type citedPath struct {
	line int
	path string
}

// sourcePathIn reduces one code span to the source path it claims, or false.
func sourcePathIn(span string) (string, bool) {
	span = strings.TrimSpace(span)
	if !pathShaped.MatchString(span) {
		return "", false
	}
	for _, ext := range sourceExtensions {
		if strings.HasSuffix(span, ext) {
			return span, !notAPathClaim(span)
		}
	}
	return "", false
}

// prosePages are the pages this gate reads: the documentation trees plus the
// rulebooks and the root prose beside them. Derived from the tree, so a new
// page under docs/ is enrolled the day it lands.
func prosePages(t *testing.T) []string {
	t.Helper()
	var pages []string
	for _, f := range trackedFiles(t) {
		if f.symlink || !strings.HasSuffix(f.path, ".md") {
			continue
		}
		// Two archives, and in both a dangling path is the POINT rather than a
		// defect: docs/evidence/ records what a review checked about code since
		// removed, banner-marked as historical on every page, and CHANGELOG.md
		// describes each change against the tree AS IT WAS. Correcting either to
		// today's paths would destroy the record it exists to keep.
		if strings.HasPrefix(f.path, "docs/evidence/") || f.path == "CHANGELOG.md" {
			continue
		}
		// Derived, not listed: everything under docs/, every rulebook wherever a
		// directory grows one, and the root prose beside them. A hand-kept list
		// here read two of the nine root pages and one of the two AGENTS.md
		// files, so DESIGN.md and SECURITY.md — which carry paths like any other
		// page — went unread while this gate reported PASS.
		switch {
		case strings.HasPrefix(f.path, "docs/"),
			filepath.Base(f.path) == "AGENTS.md",
			!strings.Contains(f.path, "/"):
			pages = append(pages, f.path)
		}
	}
	if len(pages) == 0 {
		t.Fatal("no prose pages found — this gate is reading a tree shape that is gone")
	}
	return pages
}

func TestEverySourcePathNamedInProseExists(t *testing.T) {
	t.Parallel()

	idx := indexTree(t)
	for _, page := range prosePages(t) {
		body, err := os.ReadFile(filepath.Join(repoRoot, page))
		if err != nil {
			t.Fatalf("reading %s: %v", page, err)
		}
		for _, cited := range citedSourcePaths(string(body)) {
			if !idx.resolves(cited.path) {
				t.Errorf("%s:%d names `%s`, which no file in the tree matches — a reader following this page arrives nowhere. Point it at what replaced the file, or drop the path and name the thing in prose", page, cited.line, cited.path)
			}
		}
	}
}
