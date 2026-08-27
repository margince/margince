// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package handbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docsHandbook is where the pages are authored, relative to this package.
const docsHandbook = "../../../../../docs/handbook"

// TestTheEmbeddedHandbookMatchesTheDocs fails when the authored handbook and
// the embedded copy differ, IN EITHER DIRECTION.
//
// Both directions, because the two failures are not the same failure and only
// one of them is loud. A page added to docs/ and not copied here leaves the
// product answering out of a handbook that is missing a chapter — and every
// other test still passes, because nothing else knows the page was supposed to
// exist. A page left here that docs/ no longer has is worse: the installation
// keeps citing prose that was deliberately withdrawn, and a citation is a claim
// that the quoted text is what the handbook says.
//
// The page list is derived from docs/handbook rather than named here. A gate
// that carries its own copy of its subject's contents is a second copy of the
// subject, and this one would fail short in the quietest way available: a page
// added to both sides but forgotten in the list would be compared by nothing.
//
// Run `make -C backend handbook-embed` and commit the result.
//
// Held by: TestTheEmbeddedHandbookMatchesTheDocs (backend/internal/modules/knowledge/handbook/handbook_test.go) — this test.
func TestTheEmbeddedHandbookMatchesTheDocs(t *testing.T) {
	authored, err := filepath.Glob(filepath.Join(docsHandbook, "*.md"))
	if err != nil {
		t.Fatalf("listing the authored handbook: %v", err)
	}
	// The empty case is a failure, not a vacuous pass: a wrong path here would
	// glob nothing, compare nothing and report success, which is the one way
	// this gate must not break.
	if len(authored) == 0 {
		t.Fatalf("no authored pages found under %s — this gate would compare nothing and pass", docsHandbook)
	}

	embedded, err := Pages()
	if err != nil {
		t.Fatalf("reading the embedded handbook: %v", err)
	}
	embeddedByName := make(map[string][]byte, len(embedded))
	for _, page := range embedded {
		embeddedByName[page.Filename] = page.Content
	}

	for _, path := range authored {
		name := filepath.Base(path)
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the authored %s: %v", name, err)
		}
		got, present := embeddedByName[name]
		if !present {
			t.Errorf("docs/handbook/%s is authored but not embedded — the binary would ship a handbook missing that page. "+
				"Run `make -C backend handbook-embed` and commit the result", name)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("the embedded %s differs from docs/handbook/%s. The docs/ copy is the authored one; "+
				"run `make -C backend handbook-embed` and commit the result", name, name)
		}
		delete(embeddedByName, name)
	}

	// Whatever is left was embedded and is no longer authored.
	for name := range embeddedByName {
		t.Errorf("internal/modules/knowledge/handbook/%s is embedded but no longer authored in docs/handbook — "+
			"the installation would keep citing a page that was withdrawn. "+
			"Run `make -C backend handbook-embed` and commit the result", name)
	}
}

// TestEveryEmbeddedPageCarriesProse guards the shape of the failure the gate
// above cannot see: two sides that agree on a file that is empty.
//
// rsync copying a truncated file, or a page reduced to its front matter, leaves
// both copies identical and every assertion above green, while the corpus gains
// a document that can never ground an answer and reports itself perfectly
// ingested.
func TestEveryEmbeddedPageCarriesProse(t *testing.T) {
	embedded, err := Pages()
	if err != nil {
		t.Fatalf("reading the embedded handbook: %v", err)
	}
	if len(embedded) == 0 {
		t.Fatal("no pages are embedded at all, so the shipped handbook corpus would be empty")
	}
	for _, page := range embedded {
		if len(strings.TrimSpace(string(page.Content))) == 0 {
			t.Errorf("the embedded %s holds no prose, so it would ingest to no passages", page.Filename)
		}
	}
}

// TestOpenRefusesAPathRatherThanReadingOne holds the boundary Open states: a
// filename is a page name, never a path. A document row is where the filename
// comes from, and a row is not a place this package gets to trust blindly.
func TestOpenRefusesAPathRatherThanReadingOne(t *testing.T) {
	for _, name := range []string{
		"",
		"../../../go.mod",
		"subdir/README.md",
		`..\..\go.mod`,
	} {
		if _, ok := Open(name); ok {
			t.Errorf("Open(%q) resolved to bytes; a handbook filename is a page name, not a path", name)
		}
	}
	// The positive arm in the same test, so a change that made Open refuse
	// EVERYTHING could not pass by satisfying the negatives alone.
	if _, ok := Open("README.md"); !ok {
		t.Fatal("Open could not read README.md, so the refusals above prove nothing")
	}
}
