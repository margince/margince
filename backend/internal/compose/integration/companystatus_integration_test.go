// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// company.lifecycle became company.status (the schema already carries a
// record lifecycle — archived_at, legal_hold, merged_into_id — so the bare
// word read as part of that). A saved view stores its sort key as jsonb, so
// the rename is not complete when the column moves: a view sorting on the old
// word would return rows in insertion order and report nothing wrong. This is
// the failure the migration's stored-value UPDATE exists to prevent for rows
// that already existed, and it is invisible to a column-level check.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// accountSavedViewPerms extends the account-reading rep with the saved_view
// grants the write and the read below need, without mutating the shared
// AccountRepPerms map.
func accountSavedViewPerms() principal.Permissions {
	p := AccountRepPerms
	obj := map[string]principal.ObjectGrant{}
	for k, v := range AccountRepPerms.Objects {
		obj[k] = v
	}
	obj["saved_view"] = principal.ObjectGrant{Create: true, Read: true}
	p.Objects = obj
	return p
}

// A saved view built through the real writer, not typed in by the test, so
// this proves the write-then-read path rather than only that the test can
// author jsonb.
func TestASavedViewSortingOnTheRenamedFieldStillSorts(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, accountSavedViewPerms())
	store := collections.NewStore(e.DB())

	created, err := store.CreateSavedView(rep, collections.CreateSavedViewInput{
		Resource: "companies", Name: "Sorted by status",
		Query: map[string]any{"sort": "status"},
	})
	if err != nil {
		t.Fatalf("create saved view: %v", err)
	}

	got, err := store.GetSavedView(rep, created.ID)
	if err != nil {
		t.Fatalf("get saved view: %v", err)
	}
	sort, _ := got.Query["sort"].(string)
	if sort != "status" {
		t.Fatalf("saved view sorts on %q, want %q — the rename left a stored spelling behind", sort, "status")
	}

	// The stored word is also one the list actually sorts by, not merely one
	// the jsonb round-trips: a saved view naming a sort field the vocabulary
	// no longer recognises would open onto an unsorted list with no error.
	if _, _, err := e.Contacts.ListCompanies(rep, contacts.ListCompaniesInput{Sort: &sort}); err != nil {
		t.Fatalf("ListCompanies(sort=%s): %v", sort, err)
	}
}
