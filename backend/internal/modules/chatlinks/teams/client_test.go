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

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func fixture(t *testing.T, guest bool) (*Client, *bool) {
	t.Helper()
	posted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer member-token" {
			t.Error("wrong token")
		}
		reply := ""
		switch r.URL.Path {
		case "/me/chats":
			reply = `{"value":[]}`
		case "/me":
			reply = `{"id":"U1","displayName":"Alex","userType":"Member","accountEnabled":true}`
		case "/users/U2":
			reply = `{"id":"U2","displayName":"Jules","userType":"Member","accountEnabled":true}`
		case "/me/joinedTeams":
			reply = `{"value":[{"id":"T1","displayName":"Account team"}]}`
		case "/teams/T1/channels":
			reply = `{"value":[{"id":"C1","displayName":"Pilot","membershipType":"standard"}]}`
		case "/teams/T1/channels/C1":
			reply = `{"id":"C1","displayName":"Pilot","membershipType":"standard"}`
		case "/chats/CHAT":
			reply = `{"id":"CHAT","chatType":"oneOnOne"}`
		case "/teams/T1/members", "/chats/CHAT/members":
			roles := `[]`
			if guest {
				roles = `["guest"]`
			}
			reply = `{"value":[{"userId":"U1","tenantId":"TENANT","displayName":"Alex","roles":[]},{"userId":"U2","tenantId":"TENANT","displayName":"Jules","roles":` + roles + `}]}`
		case "/chats":
			//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
			var body struct {
				Type    string        `json:"chatType"`
				Members []groupMember `json:"members"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Type != "oneOnOne" || len(body.Members) != 2 {
				t.Errorf("chat creation: %+v", body)
			}
			reply = `{"id":"CHAT"}`
		case "/chats/CHAT/messages":
			if r.Method == http.MethodPost {
				//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
				var body struct {
					Body struct {
						Type    string `json:"contentType"`
						Content string `json:"content"`
					} `json:"body"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Body.Type != "text" || body.Body.Content != "Please confirm" {
					t.Errorf("message: %+v", body)
				}
				posted = true
				reply = `{"id":"M2","createdDateTime":"2026-06-10T09:00:00Z"}`
			} else {
				reply = `{"value":[{"id":"M1","createdDateTime":"2026-06-10T08:59:00Z","from":{"user":{"id":"U2"}},"body":{"contentType":"html","content":"<p>I can send the policy.</p>"}}]}`
			}
		default:
			t.Errorf("unhandled: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if _, err := w.Write([]byte(reply)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	c := New(server.Client(), "member-token", "TENANT", "U1")
	c.http.Base = server.URL
	return c, &posted
}

func TestTeamsParticipantCanReadAndSendInTheirOwnConversation(t *testing.T) {
	c, posted := fixture(t, false)
	ctx := context.Background()
	identity, err := c.Identity(ctx)
	if err != nil || identity.Member.ID != "U1" {
		t.Fatalf("identity: %+v %v", identity, err)
	}
	conversation, err := c.OpenPeople(ctx, []string{"U2"})
	if err != nil || conversation.Kind != "direct" || len(conversation.Members) != 2 {
		t.Fatalf("chat: %+v %v", conversation, err)
	}
	page, err := c.Messages(ctx, "CHAT", "", "")
	if err != nil || len(page.Messages) != 1 || !strings.Contains(page.Messages[0].Text, "send the policy") {
		t.Fatalf("reply: %+v %v", page, err)
	}
	receipt, err := c.Send(ctx, messaging.Send{Conversation: "CHAT", Text: "Please confirm", RequestID: "attempt", ExpectedAudience: reviewed(t, c, "CHAT")})
	if err != nil || receipt.ID != "M2" || !*posted {
		t.Fatalf("send: %+v %v", receipt, err)
	}
}

func TestStandardChannelUsesTeamAudienceAndGuestBlocksInternalSend(t *testing.T) {
	c, posted := fixture(t, true)
	channels, next, err := c.Channels(context.Background(), "")
	if err != nil || len(channels) != 1 || next != "" || channels[0].ID != "T1/C1" {
		t.Fatalf("discovery: %+v %s %v", channels, next, err)
	}
	audience, err := c.Conversation(context.Background(), "T1/C1")
	if err != nil || !audience.External || !audience.AudienceKnown {
		t.Fatalf("team guest absent: %+v %v", audience, err)
	}
	_, err = c.Send(context.Background(), messaging.Send{Conversation: "T1/C1", Text: "Please confirm", RequestID: "attempt", ExpectedAudience: reviewed(t, c, "T1/C1")})
	if !errors.Is(err, messaging.ErrInvalid) || *posted {
		t.Fatalf("guest send: %v posted=%v", err, *posted)
	}
}

func TestPaginationCannotRedirectCredentialsAndChatCannotHaveChannelReplies(t *testing.T) {
	for _, cursor := range []string{"https://other.test/v1.0/users", "https://graph.microsoft.com.evil.test/v1.0/users", "http://graph.microsoft.com/v1.0/users", "https://graph.microsoft.com/beta/users", "https://user@graph.microsoft.com/v1.0/users"} {
		if _, err := nextPath(cursor); !errors.Is(err, messaging.ErrInvalid) {
			t.Fatalf("unsafe next link: %s %v", cursor, err)
		}
	}
	if _, err := messagePath("CHAT", "root"); !errors.Is(err, messaging.ErrUnsupported) {
		t.Fatalf("chat thread treated as channel: %v", err)
	}
}

func reviewed(t *testing.T, c *Client, id string) string {
	t.Helper()
	conversation, err := c.Conversation(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	key, err := messaging.AudienceKey(conversation)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
