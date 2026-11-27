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
)

func TestBotThreadRequiresCompleteStableReadsAndExplicitDeletion(t *testing.T) {
	changed, missing, deleted := false, false, false
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer graph-fixture" {
			t.Error("Graph read borrowed another credential")
		}
		var response any
		switch r.URL.Path {
		case "/teams/TEAM":
			response = map[string]any{"id": "TEAM", "displayName": "Sales"}
		case "/teams/TEAM/installedApps":
			response = map[string]any{"value": []any{map[string]any{"teamsAppDefinition": map[string]string{"azureADAppId": "APP"}}}}
		case "/teams/TEAM/channels/CHANNEL":
			response = map[string]any{"id": "CHANNEL", "membershipType": "standard"}
		case "/teams/TEAM/members":
			response = map[string]any{"value": []any{map[string]string{"userId": "U1", "tenantId": "TENANT", "displayName": "Rita Rep", "email": "rita@example.test"}}}
		case "/teams/TEAM/channels/CHANNEL/messages/ROOT":
			if missing {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			reads++
			text := "The pilot requires Legal approval."
			if changed && reads%2 == 0 {
				text = "Legal approved the pilot."
			}
			parent := map[string]any{"id": "ROOT", "from": map[string]any{"user": map[string]string{"id": "U1"}}, "createdDateTime": "2026-11-01T09:00:00Z", "body": map[string]string{"contentType": "text", "content": text}}
			if deleted {
				parent["deletedDateTime"] = "2026-11-02T09:00:00Z"
			}
			response = parent
		case "/teams/TEAM/channels/CHANNEL/messages/ROOT/replies":
			if r.URL.Query().Get("$skiptoken") == "tail" {
				response = map[string]any{"value": []any{map[string]any{"id": "REPLY", "from": map[string]any{"user": map[string]string{"id": "U1"}}, "replyToId": "ROOT", "createdDateTime": "2026-11-01T10:00:00Z", "body": map[string]string{"contentType": "html", "content": "<p>Legal has <b>not</b> approved.</p>"}}}}
			} else {
				response = map[string]any{"value": []any{}, "@odata.nextLink": "https://graph.microsoft.com/v1.0/teams/TEAM/channels/CHANNEL/messages/ROOT/replies?$skiptoken=tail"}
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	bot := NewBot(server.Client(), "graph-fixture", "connector-fixture", chatwire.BotCredential{Workspace: "TENANT", Member: "TEAM"})
	bot.graph.http.Base, bot.app = server.URL, "APP"
	ctx := context.Background()
	page, err := bot.Thread(ctx, "TEAM/CHANNEL", "ROOT", "")
	if err != nil || len(page.Messages) != 2 || page.Revision == "" || page.Next != "" {
		t.Fatalf("incomplete thread accepted: %+v %v", page, err)
	}
	for _, message := range page.Messages {
		if message.UserEmail != "rita@example.test" || message.UserName != "Rita Rep" {
			t.Fatalf("thread lost scoped author identity: %#v", message)
		}
	}
	if !strings.Contains(page.Messages[0].Text+page.Messages[1].Text, "not approved") {
		t.Fatal("lost the condition from the later replies page")
	}
	changed = true
	if _, err := bot.Thread(ctx, "TEAM/CHANNEL", "ROOT", ""); !errors.Is(err, chatwire.ErrUnreachable) {
		t.Fatalf("combined different thread versions: %v", err)
	}
	changed, missing = false, true
	if page, err := bot.Thread(ctx, "TEAM/CHANNEL", "ROOT", ""); err == nil || page.Deleted {
		t.Fatalf("missing source treated as deletion: %+v %v", page, err)
	}
	missing, deleted = false, true
	page, err = bot.Thread(ctx, "TEAM/CHANNEL", "ROOT", "")
	if err != nil || !page.Deleted {
		t.Fatalf("explicit tombstone missing: %+v %v", page, err)
	}
	for _, item := range page.Messages {
		if item.TS == "ROOT" && item.Text != "" {
			t.Fatal("deleted content left the provider boundary")
		}
	}
}

func TestGraphReplyCursorCannotReadAnotherSource(t *testing.T) {
	base := "/teams/TEAM/channels/CHANNEL/messages/ROOT/replies"
	for _, next := range []string{
		"https://other.test/v1.0" + base + "?$skiptoken=secret",
		"https://graph.microsoft.com/v1.0/teams/OTHER/channels/CHANNEL/messages/ROOT/replies?$skiptoken=secret",
		"https://graph.microsoft.com/v1.0/teams/TEAM/channels/CHANNEL/messages/OTHER/replies?$skiptoken=secret",
	} {
		if _, err := graphPagePath(base, next); err == nil {
			t.Fatalf("accepted foreign cursor %q", next)
		}
	}
}
