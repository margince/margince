// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

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

func TestParticipantUsesOwnIdentityExactGroupAudienceAndSendReceipt(t *testing.T) {
	var sent map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pat_synthetic" {
			t.Error("missing participant credential")
		}
		w.Header().Set("Content-Type", "application/json")
		body := ""
		switch r.URL.Path {
		case "/api/auth/me":
			body = `{"id":"REP","workspace_id":"TEAM","display_name":"Rita","email":"rita@example.test","is_active":true}`
		case "/api/users/REP":
			body = `{"id":"REP","workspace_id":"TEAM","display_name":"Rita","is_active":true}`
		case "/api/users/OTHER":
			body = `{"id":"OTHER","workspace_id":"TEAM","display_name":"Sam","is_active":true}`
		case "/api/dms":
			var request struct {
				People []string `json:"user_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if len(request.People) != 1 || request.People[0] != "OTHER" {
				t.Error("group request changed recipients")
			}
			body = `{"id":"018f3a1b-0000-7000-8000-00000000a001","workspace_id":"TEAM","slug":"G-pilot","type":"dm","is_member":true}`
		case "/api/channels/G-pilot", "/api/channels/id:018f3a1b-0000-7000-8000-00000000a001":
			body = `{"id":"018f3a1b-0000-7000-8000-00000000a001","workspace_id":"TEAM","slug":"G-pilot","type":"dm","is_member":true}`
		case "/api/channels/id:018f3a1b-0000-7000-8000-00000000a001/members-paginated":
			body = `{"members":[{"user_id":"REP","is_active":true},{"user_id":"OTHER","is_active":true}],"total":2}`
		case "/api/channels/id:018f3a1b-0000-7000-8000-00000000a001/messages":
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			body = `{"id":"MESSAGE","channel_id":"018f3a1b-0000-7000-8000-00000000a001","author_id":"REP","content":"Please check legal.","seq":8}`
		default:
			t.Errorf("unexpected route %s", r.URL)
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewParticipant(server.Client(), server.URL, "pat_synthetic", "TEAM", "REP")
	identity, err := client.Identity(context.Background())
	if err != nil || identity.Member.ID != "REP" {
		t.Fatalf("identity: %+v %v", identity, err)
	}
	room, err := client.OpenPeople(context.Background(), []string{"OTHER"})
	if err != nil || room.ID != "018f3a1b-0000-7000-8000-00000000a001" || !room.AudienceKnown {
		t.Fatalf("conversation: %+v %v", room, err)
	}
	audience, err := messaging.AudienceKey(room)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.Send(context.Background(), messaging.Send{Conversation: room.ID, Text: "Please check legal.", RequestID: "REQUEST", ExpectedAudience: audience})
	if err != nil || receipt.ID != "MESSAGE" || receipt.Conversation != room.ID {
		t.Fatalf("receipt: %+v %v", receipt, err)
	}
	if sent["content"] != "Please check legal." || sent["client_message_id"] != "REQUEST" {
		t.Fatalf("send: %+v", sent)
	}
	if _, err := client.OpenPeople(context.Background(), []string{"OTHER", "OTHER"}); !errors.Is(err, messaging.ErrInvalid) {
		t.Fatalf("duplicate recipient accepted: %v", err)
	}
}

func TestParticipantRefusesAnotherWorkspaceAndExternalRoster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"id":"REP","workspace_id":"OTHER","is_active":true}`
		if strings.Contains(r.URL.Path, "channels") {
			body = `{"id":"018f3a1b-0000-7000-8000-00000000a001","workspace_id":"OTHER","slug":"secret","type":"private","is_member":true}`
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewParticipant(server.Client(), server.URL, "pat_synthetic", "TEAM", "REP")
	if _, err := client.Identity(context.Background()); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("workspace mismatch: %v", err)
	}
	if _, err := client.Conversation(context.Background(), "secret"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("foreign channel: %v", err)
	}
}

func TestParticipantReadsCompletePagesAndBindsThreadRepliesToTheirChannel(t *testing.T) {
	var before string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := ""
		switch r.URL.Path {
		case "/api/channels/pilot", "/api/channels/id:018f3a1b-0000-7000-8000-00000000a001":
			body = `{"id":"018f3a1b-0000-7000-8000-00000000a001","workspace_id":"TEAM","slug":"pilot","type":"private","is_member":true}`
		case "/api/channels/id:018f3a1b-0000-7000-8000-00000000a001/messages":
			before = r.URL.Query().Get("before_seq")
			body = `{"messages":[{"id":"ROOT","workspace_id":"TEAM","channel_id":"018f3a1b-0000-7000-8000-00000000a001","author_id":"REP","content":"Can we start Monday if Legal signs?","seq":10,"created_at":"2026-06-01T10:00:00Z"}],"has_more":true}`
		case "/api/messages/ROOT":
			body = `{"id":"ROOT","workspace_id":"TEAM","channel_id":"018f3a1b-0000-7000-8000-00000000a001","thread_id":"THREAD","content":"Can we start Monday if Legal signs?","seq":10,"created_at":"2026-06-01T10:00:00Z"}`
		case "/api/messages/FOREIGN":
			body = `{"id":"FOREIGN","workspace_id":"TEAM","channel_id":"OTHER","thread_id":"THREAD"}`
		case "/api/threads/THREAD/replies":
			before = r.URL.Query().Get("before_seq")
			body = `{"parent_message":{"id":"ROOT","workspace_id":"TEAM","channel_id":"018f3a1b-0000-7000-8000-00000000a001","seq":10,"created_at":"2026-06-01T10:00:00Z"},"replies":[{"id":"REPLY","workspace_id":"TEAM","channel_id":"018f3a1b-0000-7000-8000-00000000a001","author_id":"OTHER","content":"Only after approval.","seq":12,"edited":true,"created_at":"2026-06-01T11:00:00Z"}],"has_more":true}`
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewParticipant(server.Client(), server.URL, "pat_synthetic", "TEAM", "REP")
	page, err := client.Messages(context.Background(), "pilot", "", "20")
	if err != nil || before != "20" || page.Next != "10" || len(page.Messages) != 1 || !strings.HasSuffix(page.Messages[0].Text, "if Legal signs?") {
		t.Fatalf("page lost content or cursor: %+v %v before=%s", page, err, before)
	}
	thread, err := client.Messages(context.Background(), "pilot", "ROOT", "30")
	if err != nil || before != "30" || thread.Next != "12" || len(thread.Messages) != 2 || !thread.Messages[1].Edited || thread.Messages[1].Root != "ROOT" {
		t.Fatalf("thread: %+v %v before=%s", thread, err, before)
	}
	if _, err := client.Messages(context.Background(), "pilot", "FOREIGN", ""); !errors.Is(err, messaging.ErrUnavailable) {
		t.Fatalf("foreign root opened in the current channel: %v", err)
	}
}

func TestParticipantRefusesToSendIntoAnExternalAudience(t *testing.T) {
	posted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posted = true
		}
		body := ""
		switch r.URL.Path {
		case "/api/channels/pilot", "/api/channels/id:018f3a1b-0000-7000-8000-00000000a001":
			body = `{"id":"018f3a1b-0000-7000-8000-00000000a001","workspace_id":"TEAM","slug":"pilot","type":"private","is_member":true}`
		case "/api/channels/id:018f3a1b-0000-7000-8000-00000000a001/members-paginated":
			body = `{"members":[{"user_id":"REP","is_active":true},{"user_id":"GUEST","is_active":true}],"total":2}`
		case "/api/users/REP":
			body = `{"id":"REP","workspace_id":"TEAM","is_active":true}`
		case "/api/users/GUEST":
			body = `{"id":"GUEST","workspace_id":"TEAM","is_active":true,"is_guest":true}`
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewParticipant(server.Client(), server.URL, "pat_synthetic", "TEAM", "REP")
	room, err := client.Conversation(context.Background(), "pilot")
	if err != nil || !room.External || !room.AudienceKnown {
		t.Fatalf("guest was not disclosed: %+v %v", room, err)
	}
	audience, err := messaging.AudienceKey(room)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Send(context.Background(), messaging.Send{Conversation: "pilot", RequestID: "REQUEST", Text: "Internal note", ExpectedAudience: audience}); !errors.Is(err, messaging.ErrInvalid) {
		t.Fatalf("guest send: %v", err)
	}
	if posted {
		t.Fatal("external audience reached a provider mutation")
	}
}
