// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestBotUsesOnlyItsBoundWorkspaceAndPrivateRoomAudience(t *testing.T) {
	guest := false
	participant := false
	posted := false
	unstable := false
	misrooted := false
	rootReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer bot-token" {
			t.Error("wrong bot credential")
		}
		var body string
		switch r.URL.Path {
		case "/auth.test":
			if participant {
				body = `{"ok":true,"user_id":"U_BOT","team_id":"T_ONE"}`
			} else {
				body = `{"ok":true,"user_id":"U_BOT","bot_id":"B_ONE","team_id":"T_ONE"}`
			}
		case "/conversations.info":
			body = `{"ok":true,"channel":{"id":"C_PRIVATE","is_member":true,"is_private":true}}`
		case "/conversations.members":
			body = `{"ok":true,"members":["U_BOT","U_REP"]}`
		case "/users.info":
			id := r.URL.Query().Get("user")
			encoded, err := json.Marshal(map[string]any{"ok": true, "user": map[string]any{"id": id, "team_id": "T_ONE", "real_name": "Colleague", "is_bot": id == "U_BOT", "is_restricted": guest && id == "U_REP", "profile": map[string]string{"email": "colleague@example.test"}}})
			if err != nil {
				t.Error(err)
				return
			}
			body = string(encoded)
		case "/conversations.history":
			body = `{"ok":true,"messages":[{"ts":"1795756000.123456","user":"U_REP","text":"Pilot waits for legal approval","reply_count":1,"latest_reply":"1795757000.000001"}]}`
		case "/conversations.replies":
			if r.URL.Query().Get("cursor") == "later" {
				body = `{"ok":true,"messages":[{"ts":"1795757000.000001","thread_ts":"1795756000.123456","user":"U_REP","text":"Legal approved the amended terms"}]}`
			} else {
				rootReads++
				text := "Pilot waits for legal approval"
				if unstable && rootReads%2 == 0 {
					text = "The pilot was cancelled."
				}
				threadRoot := "1795756000.123456"
				if misrooted {
					threadRoot = "1795755000.000001"
				}
				encoded, err := json.Marshal(map[string]any{"ok": true, "has_more": true, "messages": []any{map[string]string{"ts": "1795756000.123456", "thread_ts": threadRoot, "user": "U_REP", "text": text}}, "response_metadata": map[string]string{"next_cursor": "later"}})
				if err != nil {
					t.Error(err)
					return
				}
				body = string(encoded)
			}
		case "/chat.postMessage":
			posted = true
			body = `{"ok":true,"channel":"C_PRIVATE","ts":"1795758000.000001"}`
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	bot := fixtureBot(t, server.Client(), "bot-token")
	bot.client.http.Base = server.URL
	ctx := context.Background()
	if _, err := bot.Me(ctx); err != nil {
		t.Fatal(err)
	}
	roots, err := bot.Roots(ctx, "C_PRIVATE", time.Time{}, "")
	if err != nil || len(roots.Roots) != 1 || roots.Roots[0].Revision == "" {
		t.Fatalf("roots: %+v %v", roots, err)
	}
	complete, err := bot.Thread(ctx, "C_PRIVATE", roots.Roots[0].ID, "")
	if err != nil || complete.Next != "" || complete.Revision == "" || len(complete.Messages) != 2 || complete.Messages[0].UserEmail != "colleague@example.test" || complete.Messages[1].Text != "Legal approved the amended terms" {
		t.Fatalf("complete stable thread lost its tail: %+v %v", complete, err)
	}
	misrooted = true
	if _, err := bot.Thread(ctx, "C_PRIVATE", roots.Roots[0].ID, ""); !errors.Is(err, chatwire.ErrUnreachable) {
		t.Fatalf("treated a reply as its own canonical root: %v", err)
	}
	misrooted = false
	unstable = true
	if _, err := bot.Thread(ctx, "C_PRIVATE", roots.Roots[0].ID, ""); !errors.Is(err, chatwire.ErrUnreachable) {
		t.Fatalf("mixed different source versions: %v", err)
	}

	guest = true
	unstable = false
	if _, err := bot.Thread(ctx, "C_PRIVATE", roots.Roots[0].ID, ""); err != nil {
		t.Fatalf("guest membership stopped incoming source reads: %v", err)
	}
	if _, err := bot.Post(ctx, "C_PRIVATE", "Internal outcome", "request-one", ""); !errors.Is(err, chatwire.ErrAudienceNeedsReview) || posted {
		t.Fatalf("guest received internal post: %v posted=%v", err, posted)
	}
	participant = true
	if _, err := bot.Me(ctx); !errors.Is(err, chatwire.ErrTokenRejected) {
		t.Fatalf("accepted participant credential: %v", err)
	}
}

func TestBotDoesNotTreatNetworkOutageAsChannelDeletion(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		gone       bool
	}{
		{name: "semantic deletion", body: `{"ok":false,"error":"channel_not_found"}`, status: 200, gone: true},
		{name: "upstream outage", body: `{}`, status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			bot := fixtureBot(t, server.Client(), "bot-token")
			bot.client.http.Base = server.URL
			_, err := bot.Channel(context.Background(), "C_PRIVATE")
			if errors.Is(err, chatwire.ErrChannelGone) != tc.gone {
				t.Fatalf("wrong channel lifecycle error: %v", err)
			}
			if !tc.gone && !errors.Is(err, messaging.ErrUnavailable) {
				t.Fatalf("outage lost retry semantics: %v", err)
			}
		})
	}
}

func TestBotMembershipLossAndMissingRootDoNotAssertDeletion(t *testing.T) {
	readable := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"ok":false,"error":"thread_not_found"}`
		if r.URL.Path == "/conversations.info" {
			body = `{"ok":true,"channel":{"id":"C_ONE","is_private":true,"is_member":true}}`
			if r.Header.Get("Authorization") == "Bearer synthetic-bot" && !readable {
				body = `{"ok":false,"error":"channel_not_found"}`
			}
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	bot := fixtureBot(t, server.Client(), "synthetic-bot")
	bot.client.http.Base = server.URL

	page, err := bot.Thread(context.Background(), "C_ONE", "1700000000.000001", "")
	if page.Deleted || !errors.Is(err, chatwire.ErrChannelGone) {
		t.Fatalf("lost bot access became source deletion: %+v %v", page, err)
	}
	readable = true
	page, err = bot.Thread(context.Background(), "C_ONE", "1700000000.000001", "")
	if !errors.Is(err, messaging.ErrUnavailable) || page.Deleted {
		t.Fatalf("an unavailable root asserted source deletion: %+v %v", page, err)
	}
}
