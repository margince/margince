// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A claim is stored only against words the message holds: one whose quote is
// not in the cited note, or whose text is blank, would read as trustworthy as a
// grounded one.
func TestAClaimIsRecordedOnlyAgainstWordsTheNoteHolds(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var contact, note AnyMap
	e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Quoted Contact"}, nil, &contact)
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "note", "source": "manual", "body": "We will send the revised quote on Friday.",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact["id"]}},
	}, nil, &note); status != http.StatusCreated {
		t.Fatalf("log note → %d", status)
	}
	contactID, ok := contact["id"].(string)
	if !ok {
		t.Fatalf("the created contact carries no id: %v", contact)
	}
	path := "/v1/contacts/" + contactID + "/claims"
	claim := func(body, quote string) AnyMap {
		return AnyMap{"kind": "commitment_ours", "body": body, "source_activity_id": note["id"], "source_quote": quote}
	}

	expectFieldRefused(t, e, "POST", path, claim("Send the quote", "a sentence that is not in the note at all"), "source_quote")
	expectFieldRefused(t, e, "POST", path, claim("   ", "send the revised quote"), "body")
	expectFieldRefused(t, e, "POST", path, claim("Send the quote", "   "), "source_quote")

	if status := e.Call(t, "POST", path, claim("Send the quote", "send the revised  quote\non Friday"), nil, nil); status != http.StatusCreated {
		t.Errorf("a claim quoting the note → %d, want 201", status)
	}
}
