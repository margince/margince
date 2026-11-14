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
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func serve(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.Client(), srv.URL+"/", "gwapp_test")
}

func TestAPostCarriesTheClientMessageIDAndTheBotToken(t *testing.T) {
	var got map[string]any
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gwapp_test" || r.URL.Path != "/api/bot/channels/acme-deal/messages" {
			t.Errorf("request = %s %s with %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"id":"m1","seq":7}`))
	})
	posted, err := client.Post(context.Background(), "acme-deal", "hello", "post-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if posted.ID != "m1" || got["client_msg_id"] != "post-1" || got["text"] != "hello" {
		t.Fatalf("posted %+v with body %v", posted, got)
	}
	if _, threaded := got["thread_ts"]; threaded {
		t.Fatal("a post outside a thread sent a thread_ts")
	}
}

func TestASecondCreateFindsTheChannelByItsKey(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusConflict)
		case r.URL.Query().Get("external_key") == "margince:deal:1":
			_, _ = w.Write([]byte(`{"slug":"acme","name":"Acme","is_private":true,"bot_is_member":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ch, err := client.CreateChannel(context.Background(), "margince:deal:1", "Acme")
	if err != nil || ch.Slug != "acme" {
		t.Fatalf("create after a conflict = %+v, %v; want the channel found by its key", ch, err)
	}
}

func TestEachRefusalIsTyped(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrTokenRejected},
		{http.StatusForbidden, ErrNotMember},
		{http.StatusNotFound, ErrChannelGone},
		{http.StatusGone, ErrChannelGone},
		{http.StatusBadGateway, ErrUnreachable},
		{http.StatusBadRequest, ErrRequestRejected},
	}
	for _, c := range cases {
		client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(`{"error":"secret channel text"}`))
		})
		_, err := client.Channel(context.Background(), "x")
		if !errors.Is(err, c.want) {
			t.Errorf("status %d = %v, want %v", c.status, err, c.want)
		}
		if err != nil && strings.Contains(err.Error(), "secret channel text") {
			t.Errorf("status %d copied the body into the error", c.status)
		}
	}
}

func TestARateLimitNamesItsWait(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := client.Me(context.Background())
	var limited *connector.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter != 12*time.Second {
		t.Fatalf("rate limit = %v, want a RateLimitedError waiting 12s", err)
	}
}

func TestHistoryAsksForAtMostOnePage(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("after") != "41" || r.URL.Query().Get("limit") != "99" {
			t.Errorf("history query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"messages":[{"seq":42,"text":"hi"}],"next_cursor":42}`))
	})
	page, err := client.History(context.Background(), "acme", 41, 500)
	if err != nil || len(page.Messages) != 1 || page.NextCursor == nil || *page.NextCursor != 42 {
		t.Fatalf("history = %+v, %v", page, err)
	}
}
