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

	// The worker queues a send's reply, and this server runs no worker. The
	// row is written here as the worker would write it.
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO personal_channel_reply (connection_id, activity_id, delivery_key, network, thread_id, recipient_account, body)
		SELECT id, $1, 'dk-e2e', 'whatsapp', '', '84901234567@s.whatsapp.net', 'Thanks, received' FROM personal_channel_connection`, activity); err != nil {
		t.Fatal(err)
	}
	var pulled struct {
		Replies []struct {
			DeliveryID string `json:"delivery_id"`
			Attempt    int    `json:"attempt"`
			Body       string `json:"body"`
		} `json:"replies"`
		Connection struct {
			RepliesWaiting int `json:"replies_waiting"`
		} `json:"connection"`
	}
	if status := e.Call(t, "POST", "/v1/personal-channels/replies/pull", map[string]any{"max": 10}, bridge, &pulled); status != http.StatusOK ||
		len(pulled.Replies) != 1 || pulled.Replies[0].Body != "Thanks, received" || pulled.Connection.RepliesWaiting != 1 {
		t.Fatalf("pulling → %d %+v, want the member's reply", status, pulled)
	}
	delivery := pulled.Replies[0].DeliveryID
	ack := func(attempt int, outcome string) string {
		var answered struct {
			Results []struct {
				Result string `json:"result"`
			} `json:"results"`
		}
		item := map[string]any{"delivery_id": delivery, "attempt": attempt, "outcome": outcome}
		if status := e.Call(t, "POST", "/v1/personal-channels/replies/ack", map[string]any{"acks": []map[string]any{item}}, bridge, &answered); status != http.StatusOK || len(answered.Results) != 1 {
			t.Fatalf("acking → %d %+v", status, answered)
		}
		return answered.Results[0].Result
	}
	if got := ack(pulled.Replies[0].Attempt+1, "failed"); got != "superseded" {
		t.Fatalf("an ack for another attempt = %q, want superseded", got)
	}
	if got := ack(pulled.Replies[0].Attempt, "sent"); got != "recorded" {
		t.Fatalf("the ack for the attempt pulled = %q, want recorded", got)
	}
	if got := ack(pulled.Replies[0].Attempt, "sent"); got != "already_settled" {
		t.Fatalf("a second ack = %q, want already_settled", got)
	}
	// A reply the client could not deliver settles as not delivered, with its delivery.
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO comms_outbound (id, activity_id, user_id, provider, channel_user_id, body, consent_purpose, status, sent_at,
		                            recipients, cc, subject, references_chain)
		SELECT uuidv7(), $1, user_id, 'personal_channels', 'whatsapp/84901234567/' || id::text, 'Second', 'transactional', 'sent', now(),
		       NULL, NULL, NULL, NULL
		  FROM personal_channel_connection`, activity); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO personal_channel_reply (connection_id, activity_id, delivery_key, network, thread_id, recipient_account, body)
		SELECT id, $1, 'dk-e2e-2', 'whatsapp', '', '84901234567@s.whatsapp.net', 'Second' FROM personal_channel_connection`, activity); err != nil {
		t.Fatal(err)
	}
	if status := e.Call(t, "POST", "/v1/personal-channels/replies/pull", map[string]any{"max": 10}, bridge, &pulled); status != http.StatusOK || len(pulled.Replies) != 1 {
		t.Fatalf("pulling the second reply → %d %+v", status, pulled)
	}
	delivery = pulled.Replies[0].DeliveryID
	if got := ack(pulled.Replies[0].Attempt, "failed"); got != "recorded" {
		t.Fatalf("the failed ack = %q, want recorded", got)
	}
	var bounced int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM comms_outbound WHERE provider = 'personal_channels' AND bounced_at IS NOT NULL`).Scan(&bounced); err != nil || bounced != 1 {
		t.Fatalf("the failed reply's delivery bounced = %d, %v; want it marked not delivered", bounced, err)
	}
	var connection struct {
		Enabled bool `json:"enabled"`
	}
	if status := e.Call(t, "GET", "/v1/personal-channels/connection", nil, nil, &connection); status != http.StatusOK || !connection.Enabled {
		t.Fatalf("reading the connection → %d %+v", status, connection)
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

// What a client does wrong is answered, not swallowed: each refusal names the
// request's fault, and one bad message never fails the batch around it.
func TestAChatClientIsToldWhatItAskedWrong(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blobstore.NewMemory()))
	e.BootstrapWorkspace(t)
	bridge := apptest.PassportBearer(t, e, "Chat Bridge", "personal_channels")
	headers := map[string]string{"Authorization": bridge["Authorization"], "Idempotency-Key": "refusals-1"}
	push := func(msgs ...map[string]any) int {
		return e.Call(t, "POST", "/v1/personal-channels/chats", map[string]any{"messages": msgs}, headers, nil)
	}
	message := map[string]any{
		"message_id": "wamid.off", "network": "whatsapp", "body": "hi",
		"from": map[string]any{"account": "84900000001", "name": "An"}, "to": map[string]any{"account": "me"},
	}

	if status := push(message); status != http.StatusConflict {
		t.Fatalf("pushing while the connection is off → %d, want 409", status)
	}
	form, contentType := apptest.MultipartDocument(t, "a.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n"))
	if status, _ := apptest.PostUpload(t, e, "/v1/personal-channels/uploads", bridge, form, contentType); status != http.StatusConflict {
		t.Fatalf("staging a file while off → %d, want 409", status)
	}
	if status := e.Call(t, "PATCH", "/v1/personal-channels/connection", map[string]any{"enabled": "yes"}, nil, nil); status < 400 {
		t.Fatalf("a connection switch that is no boolean → %d, want a refusal", status)
	}
	if status := e.Call(t, "PATCH", "/v1/personal-channels/connection", map[string]any{"enabled": true}, nil, nil); status != http.StatusOK {
		t.Fatalf("turning on → %d", status)
	}

	for _, max := range []int{0, 51} {
		if status := e.Call(t, "POST", "/v1/personal-channels/replies/pull", map[string]any{"max": max}, bridge, nil); status < 400 {
			t.Fatalf("pulling %d replies → %d, want a refusal", max, status)
		}
	}
	for _, outcome := range []string{"lost", ""} {
		ack := map[string]any{"acks": []map[string]any{{"delivery_id": "01a1e7c4-128b-7c75-aac7-1c8828b9ccff", "outcome": outcome}}}
		if status := e.Call(t, "POST", "/v1/personal-channels/replies/ack", ack, bridge, nil); status < 400 {
			t.Fatalf("acking with the outcome %q → %d, want a refusal", outcome, status)
		}
	}
	var acked struct {
		Results []struct {
			Result string `json:"result"`
		} `json:"results"`
	}
	unknown := map[string]any{"acks": []map[string]any{{"delivery_id": "01a1e7c4-128b-7c75-aac7-1c8828b9ccff", "outcome": "sent"}}}
	if status := e.Call(t, "POST", "/v1/personal-channels/replies/ack", unknown, bridge, &acked); status != http.StatusOK ||
		len(acked.Results) != 1 || acked.Results[0].Result != "unknown_delivery" {
		t.Fatalf("acking a delivery that is not the member's → %d %+v, want unknown_delivery", status, acked)
	}

	var batch struct {
		Results []struct {
			Status string `json:"status"`
		} `json:"results"`
	}
	bad := map[string]any{"message_id": "wamid.bad", "network": "whatsapp", "body": "hi",
		"from": map[string]any{"account": "has:colon"}, "to": map[string]any{"account": "me"}}
	twice := map[string]any{"message_id": "wamid.twice", "network": "whatsapp", "body": "hi",
		"from": map[string]any{"account": "84900000002"}, "to": map[string]any{"account": "me"},
		"files": []map[string]any{{"upload_id": "01a1e7c4-128b-7c75-aac7-1c8828b9ccff"}, {"upload_id": "01a1e7c4-128b-7c75-aac7-1c8828b9ccff"}}}
	missing := map[string]any{"message_id": "wamid.missing", "network": "whatsapp", "body": "hi",
		"from": map[string]any{"account": "84900000003"}, "to": map[string]any{"account": "me"},
		"files": []map[string]any{{"upload_id": "01a1e7c4-128b-7c75-aac7-1c8828b9ccf0"}}}
	if status := e.Call(t, "POST", "/v1/personal-channels/chats", map[string]any{"messages": []map[string]any{bad, twice, missing, message}}, headers, &batch); status != http.StatusOK ||
		len(batch.Results) != 4 || batch.Results[0].Status != "refused" || batch.Results[1].Status != "refused" ||
		batch.Results[2].Status != "refused" || batch.Results[3].Status != "landed" {
		t.Fatalf("a batch with three bad messages → %d %+v, want three refused and the good one landed", status, batch)
	}
}
