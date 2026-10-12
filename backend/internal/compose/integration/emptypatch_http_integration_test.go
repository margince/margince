// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A patch that names no field is refused with a 422 naming the body, and the
// subscription keeps its version.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
)

func TestAWebhookSubscriptionPatchOfNoFieldIsRefused(t *testing.T) {
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithWebhookSigningKey(cipher))
	e.BootstrapWorkspace(t)

	var created struct {
		Subscription struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"subscription"`
	}
	if status := e.Call(t, "POST", "/v1/webhook-subscriptions", AnyMap{
		"target_url": "https://ok.example/hook", "event_types": []string{"deal.created"},
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create subscription -> %d, want 201", status)
	}
	path := "/v1/webhook-subscriptions/" + created.Subscription.ID
	headers := map[string]string{"If-Match": strconv.FormatInt(created.Subscription.Version, 10)}

	var raw json.RawMessage
	if status := e.Call(t, "PATCH", path, AnyMap{}, headers, &raw); status != http.StatusUnprocessableEntity {
		t.Fatalf("a patch of no field -> %d %s, want 422", status, raw)
	}
	if !strings.Contains(string(raw), `"field":"body"`) {
		t.Errorf("the refusal = %s, want it to name the body", raw)
	}
	var list struct {
		Data []struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/webhook-subscriptions", nil, nil, &list); status != http.StatusOK {
		t.Fatalf("list subscriptions -> %d, want 200", status)
	}
	found := false
	for _, sub := range list.Data {
		if sub.ID != created.Subscription.ID {
			continue
		}
		found = true
		if sub.Version != created.Subscription.Version {
			t.Errorf("after the refusal the subscription is at version %d, want %d", sub.Version, created.Subscription.Version)
		}
	}
	if !found {
		t.Errorf("the list holds %d subscription(s) and not the one just made", len(list.Data))
	}
}
