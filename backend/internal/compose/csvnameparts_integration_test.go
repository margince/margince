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
	cases := []struct {
		name, storedFull, wantFull string
		row                        map[string]any
	}{
		{
			"a plain join follows the new parts", "Jonh Doe", "John Doe",
			map[string]any{"first_name": "John", "last_name": "Doe"},
		},
		{
			"a hand-typed display name stays", "Dr. Jonh Doe", "Dr. Jonh Doe",
			map[string]any{"first_name": "John", "last_name": "Doe"},
		},
		{
			"a disagreeing full name cell is not sent", "Dr. Jonh Doe", "Dr. Jonh Doe",
			map[string]any{fieldFullName: "Signing-Noreply", "first_name": "John", "last_name": "Doe"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			first, last := "Jonh", "Doe"
			created, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
				FullName: tc.storedFull, FirstName: &first, LastName: &last, Source: "manual",
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			w := newCSVWriters(e.DB(), ids.NewV7(), migration.PartMapping{Object: migration.ObjectContact},
				runOptions{}, &linkResolver{sourceSystem: "csv:names"}, importColumns{})
			id := ids.UUID(created.Id)
			if _, err := w.reconcile(ctx, id, migration.Row{ExternalID: "row-1", Fields: tc.row}, nil); err != nil {
				t.Fatalf("re-import: %v", err)
			}
			got, err := e.Contacts.GetContact(ctx, ids.From[ids.ContactKind](id), storekit.LiveOnly)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if got.FullName != tc.wantFull || got.FirstName == nil || *got.FirstName != "John" {
				t.Fatalf("full_name %q, first_name %v; want %q and John", got.FullName, got.FirstName, tc.wantFull)
			}
		})
	}
}

// A re-imported first name joining a stored last name past the limit skips only
// its row, and the stored name stays. Without the NameTooLong case it fails the import.
func TestAReimportedNameJoiningPastTheLimitSkipsOnlyItsRow(t *testing.T) {
	e := integration.Setup(t)
	runID, part := ids.NewV7(), contacts.ContactNameLimit*2/3
	last := strings.Repeat("b", part)
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Ada " + last, FirstName: new("Ada"), LastName: &last, Source: "manual"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	w := newCSVWriters(e.DB(), runID, migration.PartMapping{Object: migration.ObjectContact}, runOptions{}, &linkResolver{sourceSystem: "csv:names"}, importColumns{})
	row := migration.Row{ExternalID: "row-1", Fields: map[string]any{"first_name": strings.Repeat("a", part)}}
	got, err := w.reconcile(storekit.WithImportRun(e.Admin(), runID), ids.UUID(created.Id), row, nil)
	if err != nil || !got.Skipped || got.SkipWhy.Code != migration.ReasonValueTooLong {
		t.Fatalf("overlong row = %+v, %v; want it skipped as value_too_long", got, err)
	}
	stored, err := e.Contacts.GetContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(created.Id)), storekit.LiveOnly)
	if err != nil || stored.FullName != "Ada "+last {
		t.Fatalf("stored full_name %q, %v; want it unchanged", stored.FullName, err)
	}
}
