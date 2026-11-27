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
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestImmutableMessageLookupPinsTheRoomAndCanonicalRoot(t *testing.T) {
	const room = "018f3a1b-0000-7000-8000-00000000c100"
	const selected = "018f3a1b-0000-7000-8000-00000000c101"
	const root = "018f3a1b-0000-7000-8000-00000000c102"
	mismatch, missing := false, false
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response any
		switch r.URL.Path {
		case "/api/bot/channels/id:" + room:
			response = map[string]any{"ok": true, "channel": map[string]any{"id": room, "slug": "renamed", "name": "Pilot", "bot_is_member": true}}
		case "/api/bot/channels/id:" + room + "/messages/" + selected + "/source":
			calls++
			channel := room
			if mismatch {
				channel = root
			}
			response = map[string]any{"ok": true, "channel_id": channel, "message_id": selected, "root_message_id": root}
			if missing {
				response = map[string]any{"ok": false, "error": "message_not_found"}
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
	client := NewCanonical(New(server.Client(), server.URL, "synthetic"))
	location, err := client.LocateMessage(context.Background(), room, server.URL+"/channels/id:"+room+"/messages/"+selected)
	if err != nil || location.Message != selected || location.Root != root {
		t.Fatalf("wrong immutable source: %+v %v", location, err)
	}
	if _, err := client.LocateMessage(context.Background(), room, server.URL+"/channels/foreign/messages/"+selected); !errors.Is(err, messaging.ErrInvalid) || calls != 1 {
		t.Fatalf("foreign room reached source reader: calls=%d error=%v", calls, err)
	}
	mismatch = true
	if _, err := client.LocateMessage(context.Background(), room, selected); !errors.Is(err, chatwire.ErrUnreachable) {
		t.Fatalf("accepted mismatched source: %v", err)
	}
	mismatch, missing = false, true
	if _, err := client.LocateMessage(context.Background(), room, selected); !errors.Is(err, messaging.ErrInvalid) {
		t.Fatalf("missing source not refused: %v", err)
	}
}
