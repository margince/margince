// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
)

func TestCanonicalBotBindsImmutableRoomRosterAndCompleteThreadRevisions(t *testing.T) {
	const room = "018f3a1b-0000-7000-8000-00000000c100"
	const root = "018f3a1b-0000-7000-8000-00000000c101"
	const own = "018f3a1b-0000-7000-8000-00000000c102"
	guest := false
	consent, missingScope := true, false
	revision := "snapshot-one"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/api/bot/me":
			caps := []string{"immutable_channel_ids", "canonical_threads"}
			if consent {
				caps = append(caps, "channel_members")
			}
			response = map[string]any{"ok": true, "bot_id": own, "team_id": "synthetic-team", "capabilities": caps}
		case "/api/bot/channels/old-name", "/api/bot/channels/id:" + room:
			response = map[string]any{"ok": true, "channel": map[string]any{"id": room, "slug": "renamed-room", "name": "Pilot", "is_private": true, "bot_is_member": true}}
		case "/api/bot/channels/id:" + room + "/members":
			if missingScope {
				response = map[string]any{"ok": false, "error": "missing_scope"}
				break
			}
			members := []any{map[string]any{"id": own, "display_name": "CRM bot", "is_active": true, "is_bot": true}}
			next := "people-page-two"
			if r.URL.Query().Get("cursor") != "" {
				next = ""
				members = []any{map[string]any{"id": "018f3a1b-0000-7000-8000-00000000c103", "display_name": "Jordan Lee", "email": "jordan@example.test", "is_active": true, "is_guest": guest}}
			}
			response = map[string]any{"ok": true, "channel_id": room, "revision": "audience-one", "members": members, "complete": next == "", "has_more": next != "", "response_metadata": map[string]string{"next_cursor": next}}
		case "/api/bot/messages":
			var posted struct {
				Channel string `json:"channel"`
				Root    string `json:"thread_ts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Error(err)
			}
			if posted.Channel != "id:"+room || posted.Root != "1.000001" {
				t.Errorf("immutable root replaced the provider write reference: %+v", posted)
			}
			response = map[string]any{"ok": true, "ts": "2.000001"}
		case "/api/bot/channels/id:" + room + "/threads/" + root:
			cursor := r.URL.Query().Get("cursor")
			next := "opaque-root-page-two"
			message := map[string]any{"message_id": root, "channel_id": room, "root_message_id": root, "ts": "1.000001", "thread_ts": "1.000001", "text": "The pilot is conditional on Legal approval.", "posted_at": "2026-11-10T09:00:00Z", "user_name": "Jordan Lee", "user_email": "jordan@example.test"}
			if cursor != "" {
				next = ""
				message["message_id"] = "018f3a1b-0000-7000-8000-00000000c104"
				message["ts"] = "1.000002"
				message["text"] = "Legal has not approved the agreement."
			}
			response = map[string]any{"ok": true, "channel_id": room, "root_message_id": root, "revision": revision, "messages": []any{message}, "complete": next == "", "has_more": next != "", "deleted": false, "response_metadata": map[string]string{"next_cursor": next}}
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewCanonical(New(server.Client(), server.URL, "gwapp_synthetic"))
	ctx := context.Background()
	bot, err := client.Me(ctx)
	if err != nil || !CanonicalAvailable(bot) {
		t.Fatalf("missing canonical capability proof: %+v %v", bot, err)
	}
	channel, err := client.Channel(ctx, "old-name")
	if err != nil || channel.Slug != room {
		t.Fatalf("room name became the durable key: %+v %v", channel, err)
	}
	audience, err := client.Audience(ctx, room)
	if err != nil || !audience.AudienceKnown || len(audience.Members) != 2 {
		t.Fatalf("roster incomplete: %+v %v", audience, err)
	}
	first, err := client.Thread(ctx, room, root, "")
	if err != nil || first.Next != "opaque-root-page-two" || first.Messages[0].TS != root {
		t.Fatalf("bad canonical first page: %+v %v", first, err)
	}
	second, err := client.Thread(ctx, room, root, first.Next)
	if err != nil || second.Next != "" || second.Revision != first.Revision || second.Messages[0].Text != "Legal has not approved the agreement." {
		t.Fatalf("lost complete tail or revision: %+v %v", second, err)
	}
	posted, err := client.Post(ctx, room, "Please confirm Legal approval.", "post-attempt", root)
	if err != nil || posted.ID != "2.000001" {
		t.Fatalf("native reply lost its editable legacy receipt: %+v %v", posted, err)
	}
	guest = true
	if _, err := client.Audience(ctx, room); !errors.Is(err, chatwire.ErrAudienceNeedsReview) {
		t.Fatalf("guest audience was accepted: %v", err)
	}
	guest = false
	consent = false
	if _, err := client.Audience(ctx, room); !errors.Is(err, chatwire.ErrMembersConsentRequired) || errors.Is(err, chatwire.ErrTokenRejected) {
		t.Fatalf("member consent parked a valid token: %v", err)
	}
	consent = true
	missingScope = true
	if _, err := client.Audience(ctx, room); !errors.Is(err, chatwire.ErrMembersConsentRequired) || errors.Is(err, chatwire.ErrTokenRejected) {
		t.Fatalf("missing_scope parked a valid token: %v", err)
	}
}

func TestLegacyBotDoesNotPretendToOfferCanonicalReads(t *testing.T) {
	if CanonicalAvailable(Bot{}) || CanonicalAvailable(Bot{Capabilities: []string{"immutable_channel_ids"}}) {
		t.Fatal("selected canonical reads without advertised source support")
	}
	if !CanonicalAvailable(Bot{Capabilities: []string{"immutable_channel_ids", "canonical_threads"}}) {
		t.Fatal("incoming recaps incorrectly require the independent outbound audience grant")
	}
}
