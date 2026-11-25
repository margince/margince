// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// A quoted name over two lines, or with a run of spaces, is read as one line.
// The source folds it, so every reader of the row sees that one spelling.
// Other text keeps its line breaks.
func TestAnImportedNameIsReadOnOneLine(t *testing.T) {
	ctx := context.Background()
	file := "email,full_name,title\nada@example.com,\"Ada\n  Lovelace \",\"Head of\nResearch\"\n"
	blobs := blobstore.NewMemory()
	if err := blobs.Put(ctx, "contacts.csv", strings.NewReader(file), int64(len(file)), "text/csv"); err != nil {
		t.Fatal(err)
	}
	mapping := migration.RunMapping{
		Object: migration.ObjectContact, SourceKey: "email",
		Fields: map[string]string{"email": fieldEmail, "full_name": fieldFullName, "title": fieldTitle},
	}
	source, _, err := bundleSourceFor(blobs, mapping, "contacts.csv", int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := source.Rows(ctx, migration.ObjectContact, 0, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v; want the one row", rows, err)
	}
	if got := rows[0].Fields[fieldFullName]; got != "Ada Lovelace" {
		t.Errorf("full_name = %q, want the name on one line", got)
	}
	if got := rows[0].Fields[fieldTitle]; got != "Head of\nResearch" {
		t.Errorf("title = %q, want text other than a name kept as written", got)
	}
}
