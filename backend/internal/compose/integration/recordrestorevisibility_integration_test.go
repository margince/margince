// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Undo over a change of who may read a contact.
//
// The patch that moves visibility also records the narrowing reason, so the
// audit image carries a column the update shape has no name for. These prove
// the restore puts visibility back rather than refusing over that column.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type visibleContact struct {
	ID         string `json:"id"`
	Version    int64  `json:"version"`
	Visibility string `json:"visibility"`
}

func TestEndToEnd_makingAContactPrivateGoesBack(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := createOwnedContact(t, e)
	setVisibility(t, e, contact.ID, "owner")

	undoNewestUpdate(t, e, contact.ID)

	if back := readVisibleContact(t, e, contact.ID); back.Visibility != "workspace" {
		t.Errorf("after undoing the make-private, the contact is %q, want workspace", back.Visibility)
	}
}

func TestEndToEnd_publishingAContactGoesBack(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := createOwnedContact(t, e)
	setVisibility(t, e, contact.ID, "owner")
	setVisibility(t, e, contact.ID, "workspace")

	undoNewestUpdate(t, e, contact.ID)

	if back := readVisibleContact(t, e, contact.ID); back.Visibility != "owner" {
		t.Errorf("after undoing the publish, the contact is %q, want owner", back.Visibility)
	}
}

// createOwnedContact makes a contact owned by the calling seat, since a private
// contact must name somebody who can still read it.
func createOwnedContact(t *testing.T, e *apptest.AppEnv) visibleContact {
	t.Helper()
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != 200 || me.User.ID == "" {
		t.Fatalf("GET /v1/me → %d, user id %q", status, me.User.ID)
	}
	var created visibleContact
	if status := e.Call(t, "POST", "/v1/contacts",
		AnyMap{"source": "manual", "full_name": "Vera Visible", "owner_id": me.User.ID}, nil, &created); status != 201 {
		t.Fatalf("create contact → %d", status)
	}
	return created
}

func setVisibility(t *testing.T, e *apptest.AppEnv, id, visibility string) {
	t.Helper()
	if status := e.Call(t, "PATCH", "/v1/contacts/"+id,
		AnyMap{"visibility": visibility}, nil, nil); status != 200 {
		t.Fatalf("patch visibility %s → %d", visibility, status)
	}
}

func undoNewestUpdate(t *testing.T, e *apptest.AppEnv, id string) {
	t.Helper()
	entry := theUpdateEntry(t, readHistory(t, e, "contact", id))
	if !entry.Undoable.Undoable {
		t.Fatalf("a visibility change reads as not undoable: %s", reasonOf(entry))
	}
	if status, _ := restore(t, e, "contact", id, entry.ID, readVisibleContact(t, e, id).Version); status != 200 {
		t.Fatalf("restore → %d, want 200", status)
	}
}

func readVisibleContact(t *testing.T, e *apptest.AppEnv, id string) visibleContact {
	t.Helper()
	var contact visibleContact
	if status := e.Call(t, "GET", "/v1/contacts/"+id, nil, nil, &contact); status != 200 {
		t.Fatalf("read contact → %d", status)
	}
	return contact
}
