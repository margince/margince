// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A seller reads a room's file only while they may read the message it came
// with. The file's title and filename are the message's content, so a seat
// outside its audience finds the entry absent, as it finds the message.

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASellerOutsideAMailsAudienceDoesNotSeeItsFileInTheRoom(t *testing.T) {
	blob := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blob))
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	activityID, attachmentID := captureFileOnEmail(t, e, blob, room.dealID)

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
	if status := publicCall(t, e, "GET", "/v1/public/rooms/documents/"+docID+"/file", nil, bearer(token), nil); status != http.StatusOK {
		t.Fatalf("buyer download = %d, want 200", status)
	}

	// While the mail is open, the seller sees the entry and the title the buyer took.
	if titles := sellerDocumentTitles(t, e, room.roomID); len(titles) != 1 || titles[0] != "MSA-redline.docx" {
		t.Fatalf("seller list before the narrowing = %v, want the emailed file", titles)
	}
	if count, titles := buyerDownloads(t, e, room.roomID); count != 1 || len(titles) != 1 {
		t.Fatalf("roster before the narrowing = %v downloads of %v, want 1 of the emailed file", count, titles)
	}

	// The mail becomes a teammate's, limited to its participants, and the
	// seller is not one of them.
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants', captured_by = $2 WHERE id = $1`,
		activityID, "connector:imap:"+ids.NewV7().String()); err != nil {
		t.Fatalf("narrow the mail: %v", err)
	}
	if _, err := e.Pool.Exec(context.Background(),
		`DELETE FROM activity_participant WHERE activity_id = $1`, activityID); err != nil {
		t.Fatalf("take the seller off the mail: %v", err)
	}

	if titles := sellerDocumentTitles(t, e, room.roomID); len(titles) != 0 {
		t.Errorf("seller list after the narrowing = %v, want nothing: the title and filename of a "+
			"mail this seat cannot read are its content", titles)
	}
	if count, titles := buyerDownloads(t, e, room.roomID); count != 1 || len(titles) != 0 {
		t.Errorf("roster after the narrowing = %v downloads of %v, want the count kept and no title", count, titles)
	}
	if status := e.Call(t, "DELETE", "/v1/deal-rooms/"+room.roomID+"/documents/"+docID, nil,
		map[string]string{"If-Match": fmt.Sprint(doc["version"])}, nil); status != http.StatusNotFound {
		t.Errorf("removing the entry by id after the narrowing = %d, want 404 like any absent entry", status)
	}

	// Re-opening the mail brings the entry back, so the audience is what withheld it.
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE activity SET audience = 'workspace' WHERE id = $1`, activityID); err != nil {
		t.Fatalf("re-open the mail: %v", err)
	}
	if titles := sellerDocumentTitles(t, e, room.roomID); len(titles) != 1 {
		t.Errorf("seller list after re-opening the mail = %v, want the emailed file back", titles)
	}
}

func sellerDocumentTitles(t *testing.T, e *apptest.AppEnv, roomID string) []string {
	t.Helper()
	var docs AnyMap
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+roomID+"/documents", nil, nil, &docs); status != http.StatusOK {
		t.Fatalf("seller documents = %d %v", status, docs)
	}
	list, _ := docs["data"].([]any)
	titles := make([]string, 0, len(list))
	for _, row := range list {
		entry, _ := row.(map[string]any)
		title, _ := entry["title"].(string)
		titles = append(titles, title)
	}
	return titles
}

// buyerDownloads reads the one invited buyer's row on the seller's roster.
func buyerDownloads(t *testing.T, e *apptest.AppEnv, roomID string) (float64, []any) {
	t.Helper()
	var roster AnyMap
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+roomID+"/participants", nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("roster = %d %v", status, roster)
	}
	list, _ := roster["data"].([]any)
	if len(list) != 1 {
		t.Fatalf("roster = %v, want the one invited buyer", list)
	}
	seat, _ := list[0].(map[string]any)
	count, _ := seat["download_count"].(float64)
	titles, _ := seat["documents_downloaded"].([]any)
	return count, titles
}
