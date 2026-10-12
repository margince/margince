// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// A Shortlist membership event is delivered to a subscriber who may see the
// RECORD, which says nothing about whether they may find the list. So the
// delivered body names no list at all.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
)

func TestAShortlistMembershipDeliveryNamesNoList(t *testing.T) {
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithWebhookSigningKey(cipher), compose.WithListsEnabled(true))
	e.BootstrapWorkspace(t)
	we := &webhookEnv{
		AppEnv: e, pool: e.Pool, cipher: cipher,
		wsID: apptest.InstallationWorkspaceUUID(context.Background(), t, e.Owner),
	}
	rcv := newReceiver(t, http.StatusOK)
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())
	we.createSubscription(t, rcv.server.URL+"/hook", []string{"list.member_added"})

	var list, contact integration.AnyMap
	if status := e.Call(t, "POST", "/v1/lists", integration.AnyMap{
		"name": "Private picks", "entity_type": "contact", "sharing": "private",
	}, nil, &list); status != http.StatusCreated {
		t.Fatalf("create list → %d", status)
	}
	e.Call(t, "POST", "/v1/contacts", integration.AnyMap{"source": "manual", "full_name": "Picked Contact"}, nil, &contact)
	if status := e.Call(t, "POST", "/v1/lists/"+list["id"].(string)+"/members", integration.AnyMap{
		"entity_type": "contact", "entity_id": contact["id"],
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("add member → %d", status)
	}

	var raw []byte
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT envelope FROM event_outbox WHERE envelope->>'type' = 'list.member_added'`).Scan(&raw); err != nil {
		t.Fatalf("read the staged event: %v", err)
	}
	var env kevents.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if err := deliverer.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	hits := rcv.snapshot()
	if len(hits) != 1 {
		t.Fatalf("%d deliveries, want the one to the subscriber who may see the contact", len(hits))
	}
	if bytes.Contains(hits[0].body, []byte(list["id"].(string))) || bytes.Contains(hits[0].body, []byte("list_id")) {
		t.Fatalf("the delivered body names the list: %s", hits[0].body)
	}
}
