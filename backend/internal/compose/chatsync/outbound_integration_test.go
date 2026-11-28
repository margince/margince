// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type outboundModelBoundary struct {
	requests    []model.Request
	beforeReply func()
}

func (b *outboundModelBoundary) Complete(_ context.Context, request model.Request) (model.Response, error) {
	b.requests = append(b.requests, request)
	if b.beforeReply != nil {
		b.beforeReply()
	}
	return model.Response{Text: `{"relevant":true,"reason":"approval condition","title":"Pilot approval","summary":"The pilot may start only after Legal approves. Legal has not approved the amended agreement.","decisions":[],"open_questions":["Obtain Legal approval before confirming the start."]}`}, nil
}

func TestNativeOutboundReadsTheCompleteBodyAndRechecksItBeforeDelivery(t *testing.T) {
	for _, change := range []string{"unchanged", "oversized earlier activity", "source changed", "channel audience changed", "delivery unknown"} {
		t.Run(change, func(t *testing.T) {
			e := setupFlush(t)
			e.seedRoles(t, map[ids.UUID]string{e.rep: "admin"})
			e.exec(t, `UPDATE app_user SET seat_type='full' WHERE id=$1`, e.rep)
			var connection ids.UUID
			if err := e.owner.QueryRow(context.Background(), `SELECT connection_id FROM chat_link WHERE id=$1`, e.link).Scan(&connection); err != nil {
				t.Fatal(err)
			}
			e.exec(t, `UPDATE channel_connection SET provider='slack',channel_id='T_NATIVE',address=NULL WHERE id=$1`, connection)
			e.exec(t, `UPDATE chat_link SET provider='slack',channel_slug='C_NATIVE',message_detail='full' WHERE id=$1`, e.link)
			audienceChanged := false
			posts := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var response any
				switch r.URL.Path {
				case "/auth.test":
					user := "U_BOT"
					if r.Header.Get("Authorization") == "Bearer synthetic-personal" {
						user = "U_REP"
					}
					response = map[string]any{"ok": true, "bot_id": "B_NATIVE", "user_id": user, "team_id": "T_NATIVE"}
				case "/conversations.info":
					response = map[string]any{"ok": true, "channel": map[string]any{"id": "C_NATIVE", "is_private": true, "is_member": true, "name": "Pilot"}}
				case "/conversations.members":
					members := []string{"U_BOT", "U_REP"}
					if audienceChanged {
						members = append(members, "U_OUTSIDE")
					}
					response = map[string]any{"ok": true, "members": members}
				case "/users.info":
					id := r.URL.Query().Get("user")
					email := "rep@chat.test"
					if id == "U_OUTSIDE" {
						email = "outside@example.test"
					}
					response = map[string]any{"ok": true, "user": map[string]any{"id": id, "team_id": "T_NATIVE", "is_bot": id == "U_BOT", "real_name": "Rita Rep", "profile": map[string]string{"email": email}}}
				case "/chat.postMessage":
					var sent struct {
						Text string `json:"text"`
					}
					if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
						t.Error(err)
					}
					posts = append(posts, sent.Text)
					if change == "delivery unknown" && len(posts) > 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					response = map[string]any{"ok": true, "channel": "C_NATIVE", "ts": "1795000000.000001"}
				default:
					t.Errorf("unexpected native boundary %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			base, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			now := linkedAt.Add(48 * time.Hour)
			bot := pacedSlackFixture(t, routedSlackHTTP{server.Client(), base}, "synthetic-bot", "T_NATIVE", "U_BOT", &now)
			e.flusher.Chats = nativeRootSessions{session: chatlinks.Session{Provider: "slack", ConnectionID: connection, TeamID: "T_NATIVE", Chat: bot}}
			lane := &outboundModelBoundary{}
			e.flusher.Lane = lane
			ctx := principal.WithCorrelationID(principal.WithActor(context.Background(), chatLinksPrincipal), ids.NewV7())
			source := strings.Repeat("The team reviewed operational details of the proposed pilot.\n", 130) + "FINAL CONDITION: Legal has not approved the amended agreement."
			subject := "Pilot plan and approval condition"
			store := activities.NewStore(e.db())
			if change == "oversized earlier activity" {
				huge := strings.Repeat("Complete source detail. ", 6000)
				earlier, _, err := store.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: &subject, Body: &huge, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.deal}}})
				if err != nil {
					t.Fatal(err)
				}
				e.handle(t, "activity.captured", "activity", ids.UUID(earlier.Id), `{"kind":"note"}`)
			}
			activity, _, err := store.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: &subject, Body: &source, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.deal}}})
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "source changed":
				lane.beforeReply = func() {
					replacement := "The previous source was replaced before delivery."
					if _, err := store.UpdateActivity(ctx, ids.ActivityID{UUID: ids.UUID(activity.Id)}, activities.UpdateActivityInput{Body: &replacement}); err != nil {
						t.Fatal(err)
					}
				}
			case "channel audience changed":
				lane.beforeReply = func() { audienceChanged = true }
			}
			e.handle(t, "activity.captured", "activity", ids.UUID(activity.Id), `{"kind":"note"}`)
			if change == "unchanged" {
				assertNativeOutboundPreview(t, e, lane, server)
				if len(posts) != 0 {
					t.Fatal("preview posted a message")
				}
				lane.requests = nil
			}
			err = e.flusher.Flush(ctx, e.link)
			assertOutboundHealth(t, e, change)
			if len(lane.requests) != 1 || !strings.Contains(lane.requests[0].Messages[0].Content, source) {
				t.Fatalf("complete source never reached summary engine: requests=%d err=%v", len(lane.requests), err)
			}
			if change == "oversized earlier activity" {
				var snooze *river.JobSnoozeError
				if !errors.As(err, &snooze) || len(posts) != 2 || !strings.Contains(posts[1], "Legal has not approved") {
					t.Fatalf("oversized item blocked eligible update: %v posts=%v", err, posts)
				}
				var waiting, delivered int
				if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE posted_at IS NULL AND last_error='source_too_long' AND retry_at IS NOT NULL),count(*) FILTER(WHERE posted_at IS NOT NULL) FROM chat_post_item WHERE link_id=$1`, e.link).Scan(&waiting, &delivered); err != nil {
					t.Fatal(err)
				}
				if waiting != 1 || delivered != 1 {
					t.Fatalf("wrong deferred/delivered split: %d %d", waiting, delivered)
				}
				pending, err := e.store.PendingFor(ctx, e.link)
				if err != nil || len(pending.Items) != 0 {
					t.Fatalf("deferred item retried before backoff: %#v %v", pending.Items, err)
				}
				return
			}
			if change == "delivery unknown" {
				if !errors.Is(err, messaging.ErrDeliveryUnknown) || len(posts) != 2 {
					t.Fatalf("uncertain send not reported: err=%v posts=%v", err, posts)
				}
				retryErr := e.flusher.Flush(ctx, e.link)
				if !errors.Is(retryErr, messaging.ErrDeliveryUnknown) || len(posts) != 2 || len(lane.requests) != 1 {
					t.Fatalf("uncertain post was retried: err=%v posts=%d model=%d", retryErr, len(posts), len(lane.requests))
				}
				pending, err := e.store.PendingFor(ctx, e.link)
				if err != nil {
					t.Fatal(err)
				}
				called := false
				_, err = e.store.DeliverBot(ctx, chatlinks.BotDeliveryRequest{Link: pending.Link, Key: "changed-card-snapshot", ContentHash: "new-content", AudienceHash: "same-audience"}, func(context.Context) (string, error) { called = true; return "unexpected", nil })
				if !errors.Is(err, messaging.ErrDeliveryUnknown) || called {
					t.Fatalf("another request bypassed the unresolved delivery: %v called=%v", err, called)
				}
				return
			}
			if change != "unchanged" {
				if err == nil || len(posts) != 1 {
					t.Fatalf("changed source/audience leaked a stale summary: err=%v posts=%v", err, posts)
				}
				return
			}
			if err != nil || len(posts) != 2 || !strings.Contains(posts[1], "Legal has not approved") || !strings.Contains(posts[1], "Obtain Legal approval") {
				t.Fatalf("full update remained a headline or lost conditions: err=%v posts=%v", err, posts)
			}
		})
	}
}

func assertOutboundHealth(t *testing.T, e *flushEnv, scenario string) {
	t.Helper()
	state, err := e.store.GetState(e.member(), "deal", e.deal)
	if err != nil || state.Health == nil {
		t.Fatalf("delivery health: %+v %v", state, err)
	}
	expected := ""
	switch scenario {
	case "source changed":
		expected = "source_changed"
	case "channel audience changed":
		expected = "audience_changed"
		if state.Link == nil || state.Link.Status != chatlinks.StatusActive {
			t.Fatalf("an audience review lost the link: %+v", state.Link)
		}
	case "delivery unknown":
		expected = "delivery_unknown"
	}
	actual := ""
	if state.Health.DeliveryError != nil {
		actual = *state.Health.DeliveryError
	}
	if actual != expected {
		t.Fatalf("delivery failure lost its repair reason: got %q want %q", actual, expected)
	}
}
