// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func namePtr(s string) *string { return &s }

// A display name that is the plain join of the parts follows a change to them.
// One somebody typed differently, or one the update itself changes, stays.
func TestChangingANamePartUpdatesOnlyAPlainJoinedDisplayName(t *testing.T) {
	cases := []struct {
		name         string
		storedFull   string
		update       UpdateContactInput
		wantFullName string
	}{
		{"a plain join follows the new first name", "Jonh Doe",
			UpdateContactInput{FirstName: namePtr("John")}, "John Doe"},
		{"a plain join follows a cleared last name", "Jonh Doe",
			UpdateContactInput{Clear: []string{"last_name"}}, "Jonh"},
		{"the edit form's unchanged display name is no choice", "Jonh Doe",
			UpdateContactInput{FullName: namePtr("Jonh Doe"), FirstName: namePtr("John")}, "John Doe"},
		{"a hand-typed display name stays", "Dr. Jonh Doe",
			UpdateContactInput{FirstName: namePtr("John")}, "Dr. Jonh Doe"},
		{"a display name the update sends stays", "Jonh Doe",
			UpdateContactInput{FullName: namePtr("Johnny D."), FirstName: namePtr("John")}, "Johnny D."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			created, err := e.store.CreateContact(ctx, CreateContactInput{
				FullName: tc.storedFull, FirstName: namePtr("Jonh"), LastName: namePtr("Doe"), Source: "manual",
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			got, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(created.Id)), tc.update)
			if err != nil {
				t.Fatalf("update: %v", err)
			}
			if got.FullName != tc.wantFullName {
				t.Fatalf("full_name %q, want %q", got.FullName, tc.wantFullName)
			}
		})
	}
}

// A rename made through the parts is re-checked for duplicates as a full_name
// edit is.
func TestRenamingThroughTheNamePartsQueuesThePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := e.plainContact(ctx, t, "Gerhard Sahner", "info@gerhard-sahner.test")
	second, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Max Mustermann", FirstName: namePtr("Max"), LastName: namePtr("Mustermann"), Source: "manual",
		Emails: []ContactEmailInput{{Email: "sahner@innoscripta.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("create the second record: %v", err)
	}
	secondID := ids.UUID(second.Id)
	if pairs := openCandidatePairs(ctx, t, e, "contact", secondID); len(pairs) != 0 {
		t.Fatalf("got %d pairs before the rename, want 0", len(pairs))
	}

	if _, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](secondID), UpdateContactInput{
		FirstName: namePtr("Gerhard"), LastName: namePtr("Sahner"),
	}); err != nil {
		t.Fatalf("rename through the parts: %v", err)
	}

	pairs := openCandidatePairs(ctx, t, e, "contact", secondID)
	if len(pairs) != 1 || pairs[0].OtherID != first.String() {
		t.Fatalf("got pairs %+v, want exactly the pair with the first record", pairs)
	}
}
