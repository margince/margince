// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Archiving a deal shuts its room's outside door: a room is a place to work one
// deal, so an archived deal has no room.
//
// This runs through the real stack and the public buyer routes. The question is
// which rows one SQL read admits, and a unit test over the store would have to
// state the answer it is checking.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

func TestArchivingADealShutsItsRoomToTheBuyer(t *testing.T) {
	blob := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blob))
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	_, attachmentID := captureFileOnEmail(t, e, blob, room.dealID)
	var doc AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/documents", AnyMap{
		"attachment_id": attachmentID, "group_key": "legal", "source": "manual",
	}, nil, &doc); status != http.StatusCreated {
		t.Fatalf("add a file to the room = %d %v", status, doc)
	}
	docID, _ := doc["id"].(string)

	var session AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange",
		AnyMap{"credential": room.credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, session)
	}
	token, _ := session["session_token"].(string)

	// The buyer is in, the room answers and the file downloads. Otherwise the
	// refusals below would prove nothing about the archive.
	var before AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(token), &before); status != http.StatusOK || before["access"] != "live" {
		t.Fatalf("me before the archive = %d %v, want 200 live", status, before)
	}
	if status := publicCall(t, e, "GET", "/v1/public/rooms/documents/"+docID+"/file", nil, bearer(token), nil); status != http.StatusOK {
		t.Fatalf("download before the archive = %d, want 200", status)
	}

	if status := e.Call(t, "DELETE", "/v1/deals/"+room.dealID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archiving the deal = %d, want 200", status)
	}

	// Every buyer door, because they are different routes over one read and a
	// fix wired into one of them would leave the others serving.
	for _, probe := range []struct {
		what, method, path string
		body               AnyMap
	}{
		{"the room bootstrap", "GET", "/v1/public/rooms/me", nil},
		{"the document list", "GET", "/v1/public/rooms/documents", nil},
		{"the file download", "GET", "/v1/public/rooms/documents/" + docID + "/file", nil},
		{"the thread list", "GET", "/v1/public/rooms/threads", nil},
		{"opening a thread", "POST", "/v1/public/rooms/threads", AnyMap{"body": "Still here?", "source": "manual"}},
	} {
		t.Run(probe.what, func(t *testing.T) {
			var got AnyMap
			status := publicCall(t, e, probe.method, probe.path, probe.body, bearer(token), &got)
			if status != http.StatusNotFound {
				t.Errorf("%s answered %d after the deal was archived, want 404: %v", probe.what, status, got)
			}
		})
	}
}
