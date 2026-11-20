// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A re-import that corrects the name parts follows the store's rule. A display
// name that was the plain join of the parts follows them. A hand-typed one
// stays. A full name cell that disagrees with the row's own parts is not sent.
//
// Mutation: drop withNamePartsOverStoredEmail, and the address stays the name.
func TestAReimportNamesAContactCreatedUnderItsAddress(t *testing.T) {
	e := integration.Setup(t)
	w, id := importedContact(t, e, contacts.CreateContactInput{
		FullName: "jane@acme.test", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "jane@acme.test", EmailType: contactEmailTypeWork, IsPrimary: true, Position: 1}},
	})
	ctx := storekit.WithImportRun(importAdmin(e), w.runID)
	row := migration.Row{ExternalID: importedRow, Fields: map[string]any{"first_name": "Jane", "last_name": "Doe"}}
	if _, err := w.reconcile(ctx, id, row, nil); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	got, err := e.Contacts.GetContact(ctx, ids.From[ids.ContactKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.FullName != "Jane Doe" {
		t.Fatalf("full_name %q after the re-import, want Jane Doe", got.FullName)
	}
}

func TestAReimportedNameFollowsTheStoreRule(t *testing.T) {
	long := strings.Repeat("b", contacts.ContactNameLimit*2/3)
	cases := []struct {
		name, storedFull, storedLast, wantFull, wantFirst string
		row                                               map[string]any
		skipped                                           bool
	}{
		{"a plain join follows the new parts", "Jonh Doe", "Doe", "John Doe", "John", map[string]any{"first_name": "John", "last_name": "Doe"}, false},
		{"a hand-typed display name stays", "Dr. Jonh Doe", "Doe", "Dr. Jonh Doe", "John", map[string]any{"first_name": "John", "last_name": "Doe"}, false},
		{"a disagreeing full name cell is not sent", "Dr. Jonh Doe", "Doe", "Dr. Jonh Doe", "John", map[string]any{fieldFullName: "Signing-Noreply", "first_name": "John", "last_name": "Doe"}, false},
		// Joined to the stored last name, this first name passes the limit: only its row skips.
		{"a join past the limit skips its row", "Jonh " + long, long, "Jonh " + long, "Jonh", map[string]any{"first_name": long}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			first, last := "Jonh", tc.storedLast
			w, id := importedContact(t, e, contacts.CreateContactInput{
				FullName: tc.storedFull, FirstName: &first, LastName: &last, Source: "manual",
			})
			ctx := storekit.WithImportRun(importAdmin(e), w.runID)
			done, err := w.reconcile(ctx, id, migration.Row{ExternalID: importedRow, Fields: tc.row}, nil)
			if err != nil || done.Skipped != tc.skipped {
				t.Fatalf("re-import = %+v, %v; want skipped %v", done, err, tc.skipped)
			}
			got, err := e.Contacts.GetContact(e.Admin(), ids.From[ids.ContactKind](id), storekit.LiveOnly)
			if err != nil || got.FullName != tc.wantFull || got.FirstName == nil || *got.FirstName != tc.wantFirst {
				t.Fatalf("full_name %q, first_name %v, %v; want %q and %s", got.FullName, got.FirstName, err, tc.wantFull, tc.wantFirst)
			}
		})
	}
}

const importedRow = "row-1"

// importedContact is a contact an earlier run of this source created under
// importedRow. A re-import may overwrite only such a record, or one a row names by id.
func importedContact(t *testing.T, e *integration.Env, in contacts.CreateContactInput) (*csvWriters, ids.UUID) {
	t.Helper()
	ctx := importAdmin(e)
	run, err := migration.NewRunStore(e.DB()).CreateDraftRun(ctx, migration.CreateDraftRunInput{
		SourceRef: "ws/import/" + ids.NewV7().String(), Source: "import_api", FileName: "names.csv",
		Profile: json.RawMessage(`{"files":[{"name":"names.csv","row_count":1}]}`),
		Mapping: migration.RunMapping{Object: migration.ObjectContact, Fields: map[string]string{"First": "first_name"}, SourceKey: "First"},
	})
	if err != nil {
		t.Fatalf("opening the import run: %v", err)
	}
	created, err := e.Contacts.CreateContact(e.Admin(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	w := newCSVWriters(e.DB(), run.ID, migration.PartMapping{Object: migration.ObjectContact},
		runOptions{}, &linkResolver{sourceSystem: "csv:names"}, importColumns{})
	id := ids.UUID(created.Id)
	if err := database.WithWorkspaceTx(ctx, e.DB().Pool(), func(tx pgx.Tx) error {
		return w.identities.RecordIdentityTx(ctx, tx, run.ID, w.sourceSystem, w.object, importedRow, id, migration.OriginCreated)
	}); err != nil {
		t.Fatalf("recording the import's identity: %v", err)
	}
	return w, id
}
