// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestChannelWatchReconcilesCreationRenewsAndVerifiesBeforeRemoval(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	request := WatchRequest{Channel: "TEAM/CHANNEL", URL: "https://crm.example.test/v1/channel-events/teams/graph", Secret: strings.Repeat("s", 43), Now: now}
	var current *Subscription
	creates, renewals, removals, reauth := 0, 0, 0, 0
	lost := true
	unavailable := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer graph-fixture" {
			t.Error("watch spent another credential")
		}
		w.Header().Set("Content-Type", "application/json")
		if unavailable {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var response any
		switch r.Method + " " + r.URL.Path {
		case "GET /subscriptions":
			items := []Subscription{}
			if current != nil {
				items = append(items, *current)
			}
			response = map[string]any{"value": items}
		case "POST /subscriptions":
			creates++
			var sub Subscription
			if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
				t.Error(err)
			}
			if sub.ID != "" || sub.Resource != "/teams/TEAM/channels/CHANNEL/messages" || sub.ResourceData || sub.LifecycleURL != request.URL || sub.ClientState != request.Secret || !sub.Expires.Equal(now.Add(48*time.Hour)) {
				t.Errorf("unsafe subscription request: %#v", sub)
			}
			sub.ID = "SUB"
			current = &sub
			if lost {
				lost = false
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			response = sub
		case "GET /subscriptions/SUB":
			if current == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			response = current
		case "PATCH /subscriptions/SUB":
			renewals++
			//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
			var change struct {
				Expires time.Time `json:"expirationDateTime"`
			}
			if err := json.NewDecoder(r.Body).Decode(&change); err != nil {
				t.Error(err)
			}
			current.Expires = change.Expires
			response = current
		case "POST /subscriptions/SUB/reauthorize":
			reauth++
			w.WriteHeader(http.StatusNoContent)
			return
		case "DELETE /subscriptions/SUB":
			removals++
			current = nil
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			t.Errorf("unexpected watch boundary: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	bot := NewBot(server.Client(), "graph-fixture", "connector-fixture", chatwire.BotCredential{Workspace: "TENANT", Member: "TEAM", Credential: messaging.Credential{AppID: "APP"}})
	bot.graph.http.Base = server.URL
	if _, err := bot.EnsureWatch(context.Background(), request); !errors.Is(err, messaging.ErrDeliveryUnknown) {
		t.Fatalf("lost create was not unknown: %v", err)
	}
	sub, err := bot.EnsureWatch(context.Background(), request)
	if err != nil || sub.ID != "SUB" || creates != 1 {
		t.Fatalf("retry duplicated subscription: %#v %v creates=%d", sub, err, creates)
	}
	request.Subscription = sub.ID
	request.Now = now.Add(40 * time.Hour)
	if _, err := bot.EnsureWatch(context.Background(), request); err != nil || renewals != 1 {
		t.Fatalf("watch not renewed: %v renewals=%d", err, renewals)
	}
	request.Reauthorize = true
	if _, err := bot.EnsureWatch(context.Background(), request); err != nil || reauth != 1 {
		t.Fatalf("lifecycle not reauthorized: %v reauth=%d", err, reauth)
	}
	wrong := request
	wrong.Channel = "TEAM/OTHER"
	if err := bot.RemoveWatch(context.Background(), wrong); !errors.Is(err, messaging.ErrUnauthorized) || removals != 0 {
		t.Fatalf("foreign subscription removed: %v removals=%d", err, removals)
	}
	unavailable = true
	if err := bot.RemoveWatch(context.Background(), request); err == nil {
		t.Fatal("unavailable provider was reported as confirmed removal")
	}
	unavailable = false
	if err := bot.RemoveWatch(context.Background(), request); err != nil || removals != 1 {
		t.Fatalf("owned subscription not removed: %v removals=%d", err, removals)
	}
	if err := bot.RemoveWatch(context.Background(), request); err != nil {
		t.Fatalf("confirmed absence not idempotent: %v", err)
	}
}
