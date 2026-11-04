// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
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
func TestAReimportedNameFollowsTheStoreRule(t *testing.T) {
	cases := []struct {
		name, storedFull, wantFull string
		row                        map[string]any
	}{
		{"a plain join follows the new parts", "Jonh Doe", "John Doe",
			map[string]any{"first_name": "John", "last_name": "Doe"}},
		{"a hand-typed display name stays", "Dr. Jonh Doe", "Dr. Jonh Doe",
			map[string]any{"first_name": "John", "last_name": "Doe"}},
		{"a disagreeing full name cell is not sent", "Dr. Jonh Doe", "Dr. Jonh Doe",
			map[string]any{fieldFullName: "Signing-Noreply", "first_name": "John", "last_name": "Doe"}},
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
