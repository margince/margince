// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

func TestCanonicalReadsFinishBusyThreadsAndLargeAudiencesInOneSnapshot(t *testing.T) {
	const room = "018f3a1b-0000-7000-8000-00000000c100"
	const root = "018f3a1b-0000-7000-8000-00000000c101"
	const bot = "018f3a1b-0000-7000-8000-00000000c102"
	const count = 600
	client := NewCanonical(serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response map[string]any
		switch r.URL.Path {
		case "/api/bot/me":
			response = map[string]any{"ok": true, "bot_id": bot, "team_id": "synthetic-team", "capabilities": []string{"immutable_channel_ids", "canonical_threads", "channel_members"}}
		case "/api/bot/channels/id:" + room:
			response = map[string]any{"ok": true, "channel": map[string]any{"id": room, "slug": "work-room", "is_private": true, "bot_is_member": true}}
		case "/api/bot/channels/id:" + room + "/threads/" + root, "/api/bot/channels/id:" + room + "/members":
			thread := r.URL.Path != "/api/bot/channels/id:"+room+"/members"
			if r.URL.Query().Get("cursor") != "" {
				code := "audience_changed"
				if thread {
					code = "thread_changed"
				}
				response = map[string]any{"ok": false, "error": code}
				break
			}
			limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
			if err != nil || limit < 1 {
				t.Errorf("invalid page request: %s", r.URL.RawQuery)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			next := ""
			if limit < count {
				next = "source-will-change"
			}
			items := make([]map[string]any, 0, min(limit, count))
			for i := range min(limit, count) {
				id := fmt.Sprintf("018f3a1b-0000-7000-8000-%012x", i+1)
				item := map[string]any{"id": id, "display_name": "Colleague", "is_active": true}
				if thread {
					if i == 0 {
						id = root
					}
					item = map[string]any{"message_id": id, "channel_id": room, "root_message_id": root, "ts": fmt.Sprintf("1.%06d", i+1), "thread_ts": "1.000001", "text": "Complete source", "posted_at": "2026-11-10T09:00:00Z"}
				} else if i == 0 {
					item["id"], item["is_bot"] = bot, true
				}
				items = append(items, item)
			}
			response = map[string]any{"ok": true, "channel_id": room, "revision": "current-snapshot", "complete": next == "", "has_more": next != "", "response_metadata": map[string]string{"next_cursor": next}}
			if thread {
				response["messages"], response["root_message_id"] = items, root
			} else {
				response["members"] = items
			}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	})))
	t.Run("complete thread", func(t *testing.T) {
		page, err := client.Thread(context.Background(), room, root, "")
		if err != nil || page.Next != "" || len(page.Messages) != count {
			t.Fatalf("busy thread did not finish before its next change: %d messages, next %q, error %v", len(page.Messages), page.Next, err)
		}
	})
	t.Run("complete audience", func(t *testing.T) {
		audience, err := client.Audience(context.Background(), room)
		if err != nil || !audience.AudienceKnown || len(audience.Members) != count {
			t.Fatalf("large audience did not finish before its next change: %d members, error %v", len(audience.Members), err)
		}
	})
}
