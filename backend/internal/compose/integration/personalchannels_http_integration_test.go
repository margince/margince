// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// The production wiring, end to end: a client's file lands on the message it
// carried, and the member can reply on that chat. Each half once broke only
// in the composed server, where the store-level tests could not see it.
func TestAChatClientsFileLandsAndTheMemberCanReplyOnTheChat(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blobstore.NewMemory()))
	e.BootstrapWorkspace(t)
	if status := e.Call(t, "PATCH", "/v1/personal-channels/connection", map[string]any{"enabled": true}, nil, nil); status != http.StatusOK {
		t.Fatalf("turning personal channels on → %d", status)
	}
	bridge := apptest.PassportBearer(t, e, "Chat Bridge", "personal_channels")

	form, contentType := apptest.MultipartDocument(t, "quote.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n"))
	status, raw := apptest.PostUpload(t, e, "/v1/personal-channels/uploads", bridge, form, contentType)
	var staged struct {
		UploadID string `json:"upload_id"`
	}
	if status != http.StatusCreated && status != http.StatusOK || json.Unmarshal([]byte(raw), &staged) != nil || staged.UploadID == "" {
		t.Fatalf("staging the file → %d %s", status, raw)
	}

	var pushed struct {
		Results []struct {
			Status     string `json:"status"`
			ActivityID string `json:"activity_id"`
			Reason     string `json:"reason"`
		} `json:"results"`
	}
	headers := map[string]string{"Authorization": bridge["Authorization"], "Idempotency-Key": "push-1"}
	if status := e.Call(t, "POST", "/v1/personal-channels/chats", map[string]any{"messages": []map[string]any{{
		"message_id": "wamid.e2e", "network": "whatsapp", "body": "Here is the signed quote",
		"from":  map[string]any{"account": "84901234567@s.whatsapp.net", "name": "An Tran"},
		"to":    map[string]any{"account": "me", "name": "Me"},
		"files": []map[string]any{{"upload_id": staged.UploadID}},
	}}}, headers, &pushed); status != http.StatusOK || len(pushed.Results) != 1 || pushed.Results[0].Status != "landed" {
		t.Fatalf("pushing → %d %+v", status, pushed.Results)
	}
	activity := pushed.Results[0].ActivityID

	var files int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM attachment WHERE entity_type = 'activity' AND entity_id = $1`, activity).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if files != 1 {
		t.Fatalf("%d files on the landed message, want the one it carried", files)
	}

	if status := e.Call(t, "POST", "/v1/activities/"+activity+"/send-message", map[string]any{"body": "Thanks, received"}, nil, nil); status != http.StatusAccepted {
		t.Fatalf("replying on the chat → %d, want 202 queued for the member's client", status)
	}

	if status := e.Call(t, "PATCH", "/v1/personal-channels/connection", map[string]any{"enabled": false}, nil, nil); status != http.StatusOK {
		t.Fatalf("turning personal channels off → %d", status)
	}
	var refused struct {
		Detail string `json:"detail"`
	}
	if status := e.Call(t, "POST", "/v1/personal-channels/replies/pull", map[string]any{"max": 10}, bridge, &refused); status != http.StatusConflict ||
		refused.Detail != "personal channels are off; turn them on in Settings → Connections" {
		t.Fatalf("pulling while off → %d %q, want 409 naming the switch and nothing else", status, refused.Detail)
	}
}
