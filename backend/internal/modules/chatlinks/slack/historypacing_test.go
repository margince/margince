// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func fixtureBot(t *testing.T, httpClient messaging.Doer, token string) *Bot {
	t.Helper()
	const workspace = "T_ONE"
	const member = "U_BOT"
	now := time.Unix(1, 0)
	profile, err := chatwire.SlackHistoryProfile("internal", now)
	if err != nil {
		t.Fatal(err)
	}
	meter := ratelimit.NewRegistry().NewPacer(func() time.Time { return now })
	return NewBot(httpClient, token, workspace, member).WithThreadReader(New(httpClient, token, workspace, member), nil).WithHistoryProfile(profile, "APP").WithHistoryPacing(meter, func(ctx context.Context, wait time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		now = now.Add(wait)
		return nil
	})
}

func TestHistoryPacingIsSharedAcrossReadersAndMethodsHaveSeparateBudgets(t *testing.T) {
	now := time.Unix(1, 0)
	profile, err := chatwire.SlackHistoryProfile("internal", now)
	if err != nil {
		t.Fatal(err)
	}
	meter := ratelimit.NewRegistry().NewPacer(func() time.Time { return now })
	waits := []time.Duration{}
	sleep := func(ctx context.Context, wait time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		waits = append(waits, wait)
		now = now.Add(wait)
		return nil
	}
	first := NewBot(nil, "token", "TEAM", "BOT").WithHistoryProfile(profile, "APP").WithHistoryPacing(meter, sleep)
	second := NewBot(nil, "other-token", "TEAM", "BOT").WithHistoryProfile(profile, "APP").WithHistoryPacing(meter, sleep)
	if err := first.waitHistory(context.Background(), "conversations.replies"); err != nil {
		t.Fatal(err)
	}
	if err := second.waitHistory(context.Background(), "conversations.replies"); err != nil {
		t.Fatal(err)
	}
	if err := second.waitHistory(context.Background(), "conversations.history"); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 1 || waits[0] != time.Minute/time.Duration(profile.RepliesPerMinute) {
		t.Fatalf("recreated reader reset the provider budget: %v", waits)
	}
}

func TestHistoryReadsRequireAnEligibleRecordedProfile(t *testing.T) {
	for _, distribution := range []string{"", "non_marketplace"} {
		t.Run(distribution, func(t *testing.T) {
			var profile *chatwire.HistoryProfile
			if distribution != "" {
				var err error
				profile, err = chatwire.SlackHistoryProfile(distribution, time.Unix(1, 0))
				if err != nil {
					t.Fatal(err)
				}
			}
			bot := NewBot(nil, "synthetic", "TEAM", "BOT").WithHistoryProfile(profile, "APP")
			if _, err := bot.readMessages(context.Background(), "conversations.history", url.Values{}); !errors.Is(err, chatwire.ErrHistoryProfileNeedsReview) {
				t.Fatalf("unconfirmed history profile read messages: %v", err)
			}
		})
	}
}

func TestHistoryUsesRecordedPageLimitsAndSharesProviderRetryDelay(t *testing.T) {
	now := time.Unix(1, 0)
	profile, err := chatwire.SlackHistoryProfile("internal", now)
	if err != nil {
		t.Fatal(err)
	}
	profile.HistoryPageSize, profile.RepliesPageSize = 37, 19
	var limits []string
	limited := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limits = append(limits, r.URL.Query().Get("limit"))
		expected := "Bearer synthetic"
		if r.URL.Path == "/conversations.replies" {
			expected = "Bearer synthetic-reader"
		}
		if r.Header.Get("Authorization") != expected {
			t.Errorf("wrong channel reader for %s: %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		if limited {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if _, err := w.Write([]byte(`{"ok":true,"messages":[]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	meter := ratelimit.NewRegistry().NewPacer(func() time.Time { return now })
	sleep := func(ctx context.Context, wait time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		now = now.Add(wait)
		return nil
	}
	first := NewBot(server.Client(), "synthetic", "TEAM", "BOT").WithHistoryProfile(profile, "APP").WithHistoryPacing(meter, sleep)
	first.client.http.Base = server.URL
	first.WithThreadReader(New(server.Client(), "synthetic-reader", "TEAM", "READER"), nil)
	first.threadReader.http.Base = server.URL
	for _, method := range []string{"conversations.history", "conversations.replies"} {
		if _, err := first.readMessages(context.Background(), method, url.Values{"limit": {"200"}}); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(limits, []string{"37", "19"}) {
		t.Fatalf("ignored recorded page limits: %v", limits)
	}
	limited = true
	if _, err := first.readMessages(context.Background(), "conversations.history", url.Values{}); err == nil {
		t.Fatal("accepted provider limit")
	}
	second := NewBot(nil, "other-token", "TEAM", "BOT").WithHistoryProfile(profile, "APP").WithHistoryPacing(meter, sleep)
	err = second.waitHistory(context.Background(), "conversations.history")
	var retry *connector.RateLimitedError
	if !errors.As(err, &retry) || retry.RetryAfter < 120*time.Second {
		t.Fatalf("recreated client lost Retry-After: %v", err)
	}
}
