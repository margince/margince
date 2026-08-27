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
