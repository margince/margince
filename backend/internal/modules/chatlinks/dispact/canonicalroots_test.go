// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCanonicalForwardHistoryFinishesAtItsHighWaterMark(t *testing.T) {
	const room = "018f3a1b-0000-7000-8000-00000000c100"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/bot/channels/id:" + room:
			if _, err := fmt.Fprintf(w, `{"ok":true,"channel":{"id":%q,"slug":"pilot","name":"Pilot","is_private":true,"bot_is_member":true}}`, room); err != nil {
				t.Error(err)
			}
		case "/api/bot/channels/id:" + room + "/messages":
			if _, err := fmt.Fprint(w, `{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":"42"}}`); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewCanonical(New(server.Client(), server.URL, "synthetic"))
	page, err := client.Roots(context.Background(), room, time.Time{}, "42")
	if err != nil || page.Next != "" || page.Checkpoint != "42" {
		t.Fatalf("caught-up forward history never finished: %+v %v", page, err)
	}
}
