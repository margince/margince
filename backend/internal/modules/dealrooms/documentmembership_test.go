// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rule that decides whether a file may be in a room is spelled once.
// Three readers apply it — the add, the release, the buyer's download — and
// if any of them grew its own SQL for "is this the deal's file", the next
// change to the rule would reach two of the three and the buyer would keep
// downloading a file the seller had hidden.
//
// Held by: TestRoomDocumentMembershipIsSpelledOnce (backend/internal/modules/dealrooms/documentmembership_test.go) — this test.
func TestRoomDocumentMembershipIsSpelledOnce(t *testing.T) {
	readers := map[string]bool{
		"document_write.go":         false,
		"document_read.go":          false,
		"store_public_documents.go": false,
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		if _, isReader := readers[name]; isReader {
			if !strings.Contains(text, "inTheDealsFilesArea") {
				t.Errorf("%s no longer applies inTheDealsFilesArea", name)
			}
			readers[name] = true
		} else if strings.Contains(text, "inTheDealsFilesArea") {
			t.Errorf("%s applies the membership rule; only the add, the release and the buyer download may", name)
		}
		// The spelling itself lives in document_read.go; nothing else may
		// re-derive it from the attachment's parent columns.
		if name != "document_read.go" && (strings.Contains(text, "entity_id = r.deal_id") || strings.Contains(text, "deal_document_hide")) {
			t.Errorf("%s spells the deal-file membership itself instead of applying inTheDealsFilesArea", name)
		}
	}
	for name, seen := range readers {
		if !seen {
			t.Errorf("%s was not found; if it moved, move the reader list with it", name)
		}
	}
}

// A row that never held bytes is a captured document link, and the room's
// predicate leaves it out. A Documents list admits one by its category, so the
// two rules differ by that clause and only that clause.
//
// Held here because the clause is what a tidy-up removes. The two predicates
// read alike, and sharing them would hand a buyer a link to a file nobody
// uploaded.
func TestTheRoomsFileRuleAdmitsNoRowByItsCategory(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean("document_read.go"))
	if err != nil {
		t.Fatal(err)
	}
	area := dealsFilesAreaSpelling(t, string(src))
	if strings.Contains(area, "category") {
		t.Errorf("inTheDealsFilesArea reads an attachment's category:\n%s\n\n"+
			"a Documents list admits a link row that way; a room may not, because its reader "+
			"is a buyer's download", area)
	}
	if !strings.Contains(area, "NOT a.bytes_withheld") {
		t.Error("inTheDealsFilesArea no longer refuses a row with no bytes, which is what keeps a " +
			"captured link out of a room")
	}
}

// dealsFilesAreaSpelling returns the predicate's own text, so the checks above
// read the rule rather than the whole file.
func dealsFilesAreaSpelling(t *testing.T, text string) string {
	t.Helper()
	const decl = "var inTheDealsFilesArea = `"
	start := strings.Index(text, decl)
	if start < 0 {
		t.Fatal("inTheDealsFilesArea is no longer declared as one raw string")
	}
	rest := text[start+len(decl):]
	end := strings.Index(rest, "`")
	if end < 0 {
		t.Fatal("inTheDealsFilesArea's raw string is unterminated")
	}
	return rest[:end]
}
