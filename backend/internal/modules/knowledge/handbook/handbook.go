// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package handbook is the operator handbook, carried inside the binary.
//
// The pages are AUTHORED in the repository's docs/handbook and copied here by
// `make handbook-embed`, because go:embed cannot reach outside its own package
// directory. The copy is committed. TestTheEmbeddedHandbookMatchesTheDocs is
// what stops the two drifting, and it fails in both directions.
//
// # Why the bytes live here and not in object storage
//
// This is the corpus every installation has, and object storage is optional —
// knowledge.Store treats a nil blobstore as a legitimate deployment. A handbook
// that arrived through the ordinary upload path would therefore be absent from
// exactly the installations that have nothing else in their corpus. Shipping the
// prose with the code that serves it also means the handbook cannot describe a
// release it was not built from.
package handbook

import (
	"embed"
	"io/fs"
	"iter"
	"sort"
	"strings"
)

// pages holds the handbook markdown. The pattern is explicit about the
// extension so a stray file in the directory cannot silently become a document
// an installation is told is part of its handbook.
//
//go:embed *.md
var pages embed.FS

// Page is one handbook page: the filename a reader sees in a citation, and the
// bytes that filename promises.
type Page struct {
	// Filename is the page's name in docs/handbook, unchanged. A citation shows
	// it, so it has to be the name somebody could go and open.
	Filename string
	// Content is the page's markdown, whole. A page is small — the largest here
	// is a few tens of kilobytes — so there is no reader to stream and no
	// reason to pretend otherwise.
	Content []byte
}

// Pages returns every embedded page, ordered by filename.
//
// Sorted because the seeding walks this list and an unstable order would make
// two boots of one binary write the same documents in a different sequence,
// which shows up as unexplained audit-row ordering rather than as a bug.
func Pages() ([]Page, error) {
	names, err := fs.Glob(pages, "*.md")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	out := make([]Page, 0, len(names))
	for _, name := range names {
		content, err := pages.ReadFile(name)
		if err != nil {
			return nil, err
		}
		out = append(out, Page{Filename: name, Content: content})
	}
	return out, nil
}

// Open returns one page's bytes by filename.
//
// The second return distinguishes "this build does not ship that page" from an
// empty page, which is what lets a caller holding a document row decide between
// serving nothing and reporting that the row outlived its page.
func Open(filename string) ([]byte, bool) {
	// Rejected before the read rather than sanitized: a filename carrying a
	// separator is not a handbook page under another spelling, it is a caller
	// asking for something else, and embed.FS answering "not found" for it
	// would leave that indistinguishable from a page this build dropped.
	if filename == "" || strings.ContainsAny(filename, `/\`) {
		return nil, false
	}
	content, err := pages.ReadFile(filename)
	if err != nil {
		return nil, false
	}
	return content, true
}

// Title is a page's own heading: its first `# ` line outside a code fence or an
// HTML comment. It is "" when this build does not ship the page, and
// handbook_test.go requires one on every shipped page.
func Title(filename string) string {
	content, ok := Open(filename)
	if !ok {
		return ""
	}
	return firstHeading(string(content))
}

// firstHeading is the page's first `# ` line outside fences and comments.
func firstHeading(markdown string) string {
	for _, line := range proseLines(markdown) {
		if heading, found := strings.CutPrefix(line, "# "); found {
			return strings.TrimSpace(heading)
		}
	}
	return ""
}

// proseLines yields each line outside a code fence or an HTML comment, with its
// 0-based index. A heading there is the page's; one inside is code or a note.
func proseLines(markdown string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		var fence string
		var commented bool
		for i, line := range strings.Split(markdown, "\n") {
			trimmed := strings.TrimSpace(line)
			switch {
			case commented:
				commented = !strings.Contains(trimmed, "-->")
			case fence != "":
				if closesFence(trimmed, fence) {
					fence = ""
				}
			case fenceOpening(trimmed) != "":
				fence = fenceOpening(trimmed)
			case strings.HasPrefix(trimmed, "<!--"):
				commented = !strings.Contains(trimmed, "-->")
			default:
				if !yield(i, line) {
					return
				}
			}
		}
	}
}

// fenceOpening is the run of three or more backticks or tildes that opens a
// CommonMark code fence, or "" when the line opens none.
func fenceOpening(trimmed string) string {
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return ""
	}
	run := len(trimmed) - len(strings.TrimLeft(trimmed, trimmed[:1]))
	if run < 3 {
		return ""
	}
	return trimmed[:run]
}

// closesFence holds CommonMark's rule: only the opening character, at least as
// many of it, and nothing else on the line.
func closesFence(trimmed, fence string) bool {
	return len(trimmed) >= len(fence) && strings.Trim(trimmed, fence[:1]) == ""
}
