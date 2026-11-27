// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A contact's first numbers can be taken back, as the edit dialog can remove
// every number by hand.
func TestEndToEnd_addingTheFirstNumbersIsUndoneToNone(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created contactRecord
	if status := e.Call(t, "POST", "/v1/contacts",
		AnyMap{"linkedin_url": apptest.ContactProfile(), "source": "manual", "full_name": "Greta Numbers"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/contacts/"+created.ID,
		AnyMap{"phones": []AnyMap{{"phone": "+49301234567", "phone_type": "work", "is_primary": true}}},
		nil, nil); status != 200 {
		t.Fatalf("add a number → %d", status)
	}

	entry := theUpdateEntry(t, readHistory(t, e, "contact", created.ID))
	if !entry.Undoable.Undoable {
		t.Fatalf("adding the first number reads as not undoable (%v)", reasonOf(entry))
	}
	if status, _ := restore(t, e, "contact", created.ID, entry.ID, readContact(t, e, created.ID).Version); status != 200 {
		t.Fatalf("restore → %d", status)
	}
	var after struct {
		Phones []AnyMap `json:"phones"`
	}
	if status := e.Call(t, "GET", "/v1/contacts/"+created.ID, nil, nil, &after); status != 200 {
		t.Fatalf("read contact → %d", status)
	}
	if len(after.Phones) != 0 {
		t.Errorf("phones after the undo = %v, want none", after.Phones)
	}
}
