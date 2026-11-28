// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestMessageLocationPinsTheChannelAndSelectsTheReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations.replies" || r.URL.Query().Get("channel") != "C_ONE" || r.URL.Query().Get("ts") != "1700000000.000002" {
			t.Errorf("unexpected source lookup: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"ok":true,"messages":[{"ts":"1700000000.000001","thread_ts":"1700000000.000001"}]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	bot := fixtureBot(t, server.Client(), "synthetic-token")
	bot.client.http.Base = server.URL
	if bot.threadReader != nil {
		bot.threadReader.http.Base = server.URL
	}
	for _, reference := range []string{"1700000000.000002", "https://fixture.slack.com/archives/C_ONE/p1700000000000002?thread_ts=1700000000.000001&cid=C_ONE"} {
		location, err := bot.LocateMessage(context.Background(), "C_ONE", reference)
		if err != nil || location.Message != "1700000000.000002" {
			t.Fatalf("selection lost: %+v %v", location, err)
		}
		if location.Root != "1700000000.000001" {
			t.Fatalf("reply lost its root: %+v", location)
		}
	}
	for _, reference := range []string{"https://fixture.slack.com/archives/C_OTHER/p1700000000000002", "https://fixture.slack.com/archives/C_ONE/p1700000000000002?cid=C_OTHER", "https://slack.com.attacker.test/archives/C_ONE/p1700000000000002", "not-a-timestamp"} {
		if _, err := bot.LocateMessage(context.Background(), "C_ONE", reference); !errors.Is(err, messaging.ErrInvalid) {
			t.Errorf("accepted %q: %v", reference, err)
		}
	}
}
