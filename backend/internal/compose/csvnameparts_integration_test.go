// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strconv"
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

// A re-imported first name that joins to a stored last name past the name limit
// skips its row with the length. The stored name stays, and the next row lands.
//
// Mutation: drop the NameTooLong case in reconcile, and the first row fails the import.
func TestAReimportedNameJoiningPastTheLimitSkipsOnlyItsRow(t *testing.T) {
	e := integration.Setup(t)
	runID := ids.NewV7()
	ctx := storekit.WithImportRun(e.Admin(), runID)
	part := contacts.ContactNameLimit * 2 / 3
	long, short := strings.Repeat("b", part), "Doe"
	var stored [2]ids.UUID
	for i, last := range []string{long, short} {
		created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: "Ada " + last, FirstName: new("Ada"), LastName: &last, Source: "manual",
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		stored[i] = ids.UUID(created.Id)
	}
	w := newCSVWriters(e.DB(), runID, migration.PartMapping{Object: migration.ObjectContact},
		runOptions{}, &linkResolver{sourceSystem: "csv:names"}, importColumns{})
	refused, err := w.reconcile(ctx, stored[0], migration.Row{ExternalID: "row-1", Fields: map[string]any{"first_name": strings.Repeat("a", part)}}, nil)
	if err != nil {
		t.Fatalf("the overlong row failed the import: %v", err)
	}
	if !refused.Skipped || refused.SkipWhy.Code != migration.ReasonValueTooLong || !strings.Contains(refused.SkipReason, strconv.Itoa(2*part+1)) {
		t.Fatalf("overlong row = %+v, want it skipped as value_too_long at %d", refused, 2*part+1)
	}
	if _, err := w.reconcile(ctx, stored[1], migration.Row{ExternalID: "row-2", Fields: map[string]any{"first_name": "John"}}, nil); err != nil {
		t.Fatalf("next row: %v", err)
	}
	for i, want := range []string{"Ada " + long, "John Doe"} {
		got, err := e.Contacts.GetContact(e.Admin(), ids.From[ids.ContactKind](stored[i]), storekit.LiveOnly)
		if err != nil || got.FullName != want {
			t.Fatalf("contact %d full_name %q, %v; want %q", i, got.FullName, err, want)
		}
	}
}
