// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Narrowing a message's audience takes its file out of the room.
//
// The add-time check asks whether the SELLER may read the carrying message, and
// that answer was the only one ever asked: a file added while the mail was open
// to the workspace went on being listed and downloaded by an external buyer
// after the company limited it. The membership predicate already re-checks
// archived, unlinked and hidden on every read; the audience was the one term
// missing from it.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

func TestNarrowingAMailsAudienceTakesItsFileOutOfTheRoom(t *testing.T) {
	blob := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blob))
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	var roomRow AnyMap
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID, nil, nil, &roomRow); status != http.StatusOK {
		t.Fatalf("room = %d", status)
	}
	dealID, _ := roomRow["deal_id"].(string)
	activityID, attachmentID := captureFileOnEmail(t, e, blob, dealID)

	// Shared while the mail is open to the workspace, which is the state the
	// add-time check admits and the only one it ever sees.
	var doc AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/documents", AnyMap{
		"attachment_id": attachmentID, "group_key": "legal", "source": "manual",
	}, nil, &doc); status != http.StatusCreated {
		t.Fatalf("add the emailed file = %d %v", status, doc)
	}
	docID, _ := doc["id"].(string)

	var session AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange",
		AnyMap{"credential": room.credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d", status)
	}
	token, _ := session["session_token"].(string)

	// The buyer holds it — otherwise the refusals below prove nothing.
	if status := publicCall(t, e, "GET", "/v1/public/rooms/documents/"+docID+"/file", nil, bearer(token), nil); status != http.StatusOK {
		t.Fatalf("buyer download before the narrowing = %d, want 200", status)
	}

	// The company limits the correspondence afterwards.
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, activityID); err != nil {
		t.Fatalf("narrow the mail: %v", err)
	}

	if status := publicCall(t, e, "GET", "/v1/public/rooms/documents/"+docID+"/file", nil, bearer(token), nil); status != http.StatusNotFound {
		t.Fatalf("buyer download after the narrowing = %d, want 404 — content the company limited is "+
			"still reaching an external party", status)
	}
	var buyerDocs AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/documents", nil, bearer(token), &buyerDocs); status != http.StatusOK {
		t.Fatalf("buyer documents = %d", status)
	}
	if list, _ := buyerDocs["data"].([]any); len(list) != 0 {
		t.Fatalf("the limited mail's file is still in the release: %v", list)
	}
}
