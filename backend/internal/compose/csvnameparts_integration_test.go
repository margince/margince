// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
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
	ctx := e.Admin()
	created, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
		FullName: "jane@acme.test", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "jane@acme.test", EmailType: contactEmailTypeWork, IsPrimary: true, Position: 1}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	w := newCSVWriters(e.DB(), ids.NewV7(), migration.PartMapping{Object: migration.ObjectContact},
		runOptions{}, &linkResolver{sourceSystem: "csv:names"}, importColumns{})
	id := ids.UUID(created.Id)
	row := migration.Row{ExternalID: "row-1", Fields: map[string]any{"first_name": "Jane", "last_name": "Doe"}}
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
			e, runID := integration.Setup(t), ids.NewV7()
			ctx := storekit.WithImportRun(e.Admin(), runID)
			first, last := "Jonh", tc.storedLast
			created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
				FullName: tc.storedFull, FirstName: &first, LastName: &last, Source: "manual",
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			w := newCSVWriters(e.DB(), runID, migration.PartMapping{Object: migration.ObjectContact},
				runOptions{}, &linkResolver{sourceSystem: "csv:names"}, importColumns{})
			id := ids.UUID(created.Id)
			done, err := w.reconcile(ctx, id, migration.Row{ExternalID: "row-1", Fields: tc.row}, nil)
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
