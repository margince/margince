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

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestBotUsesConnectorReferencesAndNeverAppOnlyGraphMessageWrites(t *testing.T) {
	private, unknown, denied := false, false, false
	writes := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch {
		case strings.HasPrefix(r.URL.Path, "/v3/"):
			if r.Header.Get("Authorization") != "Bearer connector-fixture" {
				t.Error("connector received Graph or human token")
			}
			writes = append(writes, r.Method+" "+r.URL.Path)
			if unknown {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
			var request struct {
				Text       string `json:"text"`
				TextFormat string `json:"textFormat"`
				Activity   struct {
					Text       string `json:"text"`
					TextFormat string `json:"textFormat"`
				} `json:"activity"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.TextFormat != "plain" && request.Activity.TextFormat != "plain" {
				t.Error("CRM text could create native markup or mentions")
			}
			response = map[string]string{"id": "opaque-conversation", "activityId": "native-activity"}
			if r.URL.Path != "/v3/conversations" {
				response = map[string]string{"id": "native-reply"}
			}
		default:
			if r.Header.Get("Authorization") != "Bearer graph-fixture" || r.Method != http.MethodGet {
				t.Error("Graph app access attempted a write or used a personal token")
			}
			switch r.URL.Path {
			case "/teams/TEAM":
				response = map[string]string{"id": "TEAM", "displayName": "Sales"}
			case "/teams/TEAM/installedApps":
				response = map[string]any{"value": []any{map[string]any{"teamsAppDefinition": map[string]string{"azureADAppId": "APP"}}}}
			case "/teams/TEAM/channels/CHANNEL":
				kind := "standard"
				if private {
					kind = "private"
				}
				response = map[string]string{"id": "CHANNEL", "membershipType": kind}
			case "/teams/TEAM/channels/CHANNEL/messages":
				if denied {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				response = map[string][]string{"value": {}}
			case "/teams/TEAM/members", "/teams/TEAM/channels/CHANNEL/members", "/teams/TEAM/channels/CHANNEL/allMembers":
				response = map[string]any{"value": []any{map[string]string{"userId": "U1", "tenantId": "TENANT", "email": "jordan@example.test"}}}
			default:
				t.Errorf("unexpected Graph resource %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	credential := chatwire.BotCredential{Workspace: "TENANT", Member: "TEAM", NativeTeam: "native-team", BotMember: "native-bot", Credential: messaging.Credential{AppID: "APP"}}
	bot := NewBot(server.Client(), "graph-fixture", "connector-fixture", credential)
	bot.graph.http.Base, bot.connector.Base = server.URL, server.URL
	ctx := context.Background()
	posted, err := bot.Post(ctx, "TEAM/CHANNEL", "Legal has not approved the pilot.", "attempt", "")
	if err != nil || posted.ID == "" || len(writes) != 1 {
		t.Fatalf("native post failed: %+v %v writes=%v", posted, err, writes)
	}
	if err := bot.Update(ctx, "TEAM/CHANNEL", posted.ID, "Legal approved the amended agreement."); err != nil || len(writes) != 2 || writes[1] != "PUT /v3/conversations/opaque-conversation/activities/native-activity" {
		t.Fatalf("lost native edit reference: %v writes=%v", err, writes)
	}
	if _, err := bot.Post(ctx, "TEAM/CHANNEL", "Quick follow-up", "reply-attempt", "unobserved-root"); !errors.Is(err, messaging.ErrUnavailable) || len(writes) != 2 {
		t.Fatalf("guessed an unverified conversation reference: %v writes=%v", err, writes)
	}
	bot.WithThreadReferences(func(_ context.Context, id, root string) (ConversationReference, error) {
		return ConversationReference{Conversation: "signed-reference", Activity: root, Root: root, Channel: id, App: "APP", Tenant: "TENANT"}, nil
	})
	if _, err := bot.Post(ctx, "TEAM/CHANNEL", "Quick follow-up", "reply-attempt", "verified-root"); err != nil || len(writes) != 3 || writes[2] != "POST /v3/conversations/signed-reference/activities/verified-root" {
		t.Fatalf("verified reply failed: %v writes=%v", err, writes)
	}
	private = true
	if _, err := bot.Post(ctx, "TEAM/CHANNEL", "Private update", "private-attempt", ""); err != nil || len(writes) != 4 {
		t.Fatalf("qualified private-channel delivery failed: %v writes=%v", err, writes)
	}
	denied = true
	if _, err := bot.Post(ctx, "TEAM/CHANNEL", "Denied private update", "denied-attempt", ""); !errors.Is(err, chatwire.ErrNotMember) || len(writes) != 4 {
		t.Fatalf("private channel without app access was posted: %v %v", err, writes)
	}
	private, unknown = false, true
	if _, err := bot.Post(ctx, "TEAM/CHANNEL", "Uncertain update", "unknown-attempt", ""); !errors.Is(err, messaging.ErrDeliveryUnknown) || len(writes) != 5 {
		t.Fatalf("uncertain delivery was treated as safe to retry: %v writes=%v", err, writes)
	}
}
