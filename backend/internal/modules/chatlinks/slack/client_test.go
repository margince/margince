// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
		if r.Header.Get("Authorization") != "Bearer participant-token" {
			t.Error("participant credential missing")
		}
		reply := ""
		switch r.URL.Path {
		case "/auth.test":
			reply = `{"ok":true,"user_id":"U1","team_id":"T1"}`
		case "/users.info":
			id := r.URL.Query().Get("user")
			restricted := "false"
			if guest && id == "U2" {
				restricted = "true"
			}
			reply = `{"ok":true,"user":{"id":"` + id + `","team_id":"T1","real_name":"Colleague","is_restricted":` + restricted + `}}`
		case "/conversations.info":
			reply = `{"ok":true,"channel":{"id":"D1","is_im":true}}`
		case "/conversations.members":
			reply = `{"ok":true,"members":["U1","U2"]}`
		case "/conversations.open":
			reply = `{"ok":true,"channel":{"id":"D1"}}`
		case "/conversations.history", "/conversations.replies":
			reply = `{"ok":true,"messages":[{"ts":"1795756000.123456","thread_ts":"1795755000.000001","user":"U2","text":"Only this participant can see this"}]}`
		case "/chat.postMessage":
			var body struct {
				Channel  string `json:"channel"`
				Text     string `json:"text"`
				Parse    string `json:"parse"`
				Client   string `json:"client_msg_id"`
				Markdown bool   `json:"mrkdwn"`
				Unfurl   bool   `json:"unfurl_links"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Channel != "D1" || body.Client == "" || body.Parse != "none" || body.Markdown || body.Unfurl {
				t.Errorf("unsafe message body: %+v", body)
			}
			posted = true
			reply = `{"ok":true,"channel":"D1","ts":"1795756000.123456"}`
		default:
			t.Errorf("unhandled endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if _, err := w.Write([]byte(reply)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client := New(server.Client(), "participant-token", "T1", "U1")
	client.http.Base = server.URL
	return client, &posted
}

func TestParticipantConversationReadsAndSendsWithVerifiedRoster(t *testing.T) {
	client, posted := fixture(t, false)
	ctx := context.Background()
	identity, err := client.Identity(ctx)
	if err != nil || identity.Member.ID != "U1" || identity.Workspace != "T1" {
		t.Fatalf("identity: %+v %v", identity, err)
	}
	conversation, err := client.OpenPeople(ctx, []string{"U2"})
	if err != nil || conversation.ID != "D1" || !conversation.AudienceKnown || len(conversation.Members) != 2 {
		t.Fatalf("audience: %+v %v", conversation, err)
	}
	page, err := client.Messages(ctx, "D1", "1795755000.000001", "")
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Author != "U2" || page.Messages[0].At.Nanosecond() != 123456000 {
		t.Fatalf("private reply: %+v %v", page, err)
	}
	receipt, err := client.Send(ctx, messaging.Send{Conversation: "D1", Text: "Can you confirm?", RequestID: "stable-attempt", ExpectedAudience: reviewed(t, client, "D1")})
	if err != nil || receipt.ID == "" || !*posted {
		t.Fatalf("receipt: %+v %v", receipt, err)
	}
}

func TestPrivateSendRefusesAnExternalParticipantBeforePosting(t *testing.T) {
	client, posted := fixture(t, true)
	_, err := client.Send(context.Background(), messaging.Send{Conversation: "D1", Text: "Internal context", RequestID: "attempt", ExpectedAudience: reviewed(t, client, "D1")})
	if !errors.Is(err, messaging.ErrInvalid) || *posted {
		t.Fatalf("external recipient accepted: %v posted=%v", err, *posted)
	}
}

func TestTooLargeAndRepeatedRostersCreateNoConversation(t *testing.T) {
	client, posted := fixture(t, false)
	for _, members := range [][]string{{"U2", "U2"}, {"U1"}, {"U2", "U3", "U4", "U5", "U6", "U7", "U8", "U9", "U10"}} {
		_, err := client.OpenPeople(context.Background(), members)
		if err == nil {
			t.Fatalf("invalid roster accepted: %v", members)
		}
	}
	if *posted {
		t.Fatal("invalid roster posted")
	}
}

func TestIdentityMismatchAndUnknownDeliveryStayExplicit(t *testing.T) {
	client, _ := fixture(t, false)
	client.workspace = "OTHER"
	if _, err := client.Identity(context.Background()); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("wrong workspace accepted: %v", err)
	}

	client.workspace = "T1"
	original := client.http.Client
	client.http.Client = requestDoer(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"ok":true,"channel":"OTHER","ts":"1.000001"}`))}, nil
		}
		return original.Do(r)
	})
	_, err := client.Send(context.Background(), messaging.Send{Conversation: "D1", Text: "Hello", RequestID: "attempt", ExpectedAudience: reviewed(t, client, "D1")})
	if !errors.Is(err, messaging.ErrDeliveryUnknown) {
		t.Fatalf("unverified receipt accepted: %v", err)
	}
}

type requestDoer func(*http.Request) (*http.Response, error)

func (f requestDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

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

func TestThreadReplyPagesDeclareThatTheirCursorLeadsToLaterReplies(t *testing.T) {
	client, _ := fixture(t, false)
	original := client.http.Client
	client.http.Client = requestDoer(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/conversations.replies") {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"ok":true,"messages":[{"ts":"1.000001","user":"U1","text":"Question"}],"has_more":true,"response_metadata":{"next_cursor":"LATER"}}`))}, nil
		}
		return original.Do(r)
	})
	page, err := client.Messages(context.Background(), "D1", "1.000001", "")
	if err != nil || page.Next != "LATER" || page.NextDirection != "newer" {
		t.Fatalf("reply pagination misrepresented as older history: %+v %v", page, err)
	}
}
