// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// meteredSpec is a chassis edge with a fixed secret, the given rate, and a
// handler that counts how many requests reached it.
func meteredSpec(rate *WebhookRate, reached *int) WebhookSpec {
	return WebhookSpec{
		Provider: "test",
		MaxBody:  1 << 10,
		Secret:   func(*http.Request) (string, string) { return "s", "s" },
		Handle: func(context.Context, *http.Request, []byte) (Disposition, error) {
			*reached++
			return Accepted, nil
		},
		OnAccept: http.StatusOK,
		Rate:     rate,
	}
}

func postTo(handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/webhooks/test", strings.NewReader("{}"))
	r.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// An unmetered edge is every edge that existed before the stage.
func TestChassisWithNoRateAdmitsEverything(t *testing.T) {
	reached := 0
	h := Webhook(meteredSpec(nil, &reached), quietLog())
	for range 5 {
		if got := postTo(h, "10.0.0.1:1234").Code; got != http.StatusOK {
			t.Fatalf("unmetered edge answered %d", got)
		}
	}
	if reached != 5 {
		t.Fatalf("handler reached %d times, want 5", reached)
	}
}

func TestChassisMetersPerIP(t *testing.T) {
	reached := 0
	h := Webhook(meteredSpec(&WebhookRate{PerIP: ratelimit.New(2, time.Minute)}, &reached), quietLog())
	for i := range 3 {
		got := postTo(h, "10.0.0.1:1234").Code
		want := http.StatusOK
		if i == 2 {
			want = http.StatusTooManyRequests
		}
		if got != want {
			t.Fatalf("request %d answered %d, want %d", i, got, want)
		}
	}
	if reached != 2 {
		t.Fatalf("handler reached %d times, want 2 — the third should not have been served", reached)
	}
}

// A second address has its own budget, or one noisy sender would lock out
// everybody else.
func TestChassisMetersEachAddressSeparately(t *testing.T) {
	reached := 0
	h := Webhook(meteredSpec(&WebhookRate{PerIP: ratelimit.New(1, time.Minute)}, &reached), quietLog())
	if got := postTo(h, "10.0.0.1:1234").Code; got != http.StatusOK {
		t.Fatalf("first address answered %d", got)
	}
	if got := postTo(h, "10.0.0.2:1234").Code; got != http.StatusOK {
		t.Fatalf("a second address paid the first's budget: %d", got)
	}
}

// THE ONE THAT MATTERS: ratelimit leaves an over-long key unmetered and
// admitted, by design. An edge keyed on a caller-chosen value must therefore
// still be braked by the bounded client-IP bucket — otherwise a long key is a
// self-serve way off the meter.
func TestChassisOverLongEndpointKeyDoesNotBypassTheIPBucket(t *testing.T) {
	reached := 0
	rate := &WebhookRate{
		PerIP:       ratelimit.New(2, time.Minute),
		PerEndpoint: ratelimit.New(1, time.Minute),
		// Longer than ratelimit's own bound, which makes it unmeterable.
		EndpointKey: func(*http.Request) string { return strings.Repeat("k", 4096) },
	}
	h := Webhook(meteredSpec(rate, &reached), quietLog())
	codes := []int{}
	for range 3 {
		codes = append(codes, postTo(h, "10.0.0.1:1234").Code)
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("an over-long endpoint key rode past the meter: %v", codes)
	}
}

// Both budgets are spent even when the first refuses, or a flood from one
// address would leave the endpoint's own bucket reporting a quiet edge.
func TestChassisSpendsBothBudgetsOnARefusal(t *testing.T) {
	reached := 0
	endpoint := ratelimit.New(2, time.Minute)
	rate := &WebhookRate{
		PerIP:       ratelimit.New(1, time.Minute),
		PerEndpoint: endpoint,
		EndpointKey: func(*http.Request) string { return "e" },
	}
	h := Webhook(meteredSpec(rate, &reached), quietLog())
	postTo(h, "10.0.0.1:1234")
	postTo(h, "10.0.0.1:1234") // refused by the IP bucket
	// The endpoint bucket has now seen two, so a request from a FRESH address
	// is refused by the endpoint bucket alone.
	if got := postTo(h, "10.0.0.9:1234").Code; got != http.StatusTooManyRequests {
		t.Fatalf("the endpoint bucket did not count traffic the IP bucket refused: %d", got)
	}
}

func freshSpec(now time.Time, skew time.Duration, reached *int) WebhookSpec {
	spec := meteredSpec(nil, reached)
	spec.Fresh = &WebhookFreshness{
		At: func(r *http.Request) (time.Time, bool) {
			secs, err := strconv.ParseInt(r.Header.Get("X-Test-Timestamp"), 10, 64)
			if err != nil {
				return time.Time{}, false
			}
			return time.Unix(secs, 0), true
		},
		Skew: skew,
		Now:  func() time.Time { return now },
	}
	return spec
}

func postAt(handler http.Handler, stamp string) int {
	r := httptest.NewRequest(http.MethodPost, "/webhooks/test", strings.NewReader("{}"))
	r.RemoteAddr = "10.0.0.1:1234"
	if stamp != "" {
		r.Header.Set("X-Test-Timestamp", stamp)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w.Code
}

func TestChassisFreshness(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name  string
		stamp string
		want  int
	}{
		{"current", strconv.FormatInt(now.Unix(), 10), http.StatusOK},
		{"just inside the window", strconv.FormatInt(now.Add(-4*time.Minute).Unix(), 10), http.StatusOK},
		{"exactly at the bound is fresh", strconv.FormatInt(now.Add(-5*time.Minute).Unix(), 10), http.StatusOK},
		{"stale", strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10), http.StatusUnauthorized},
		// A sender with a fast clock would otherwise mint requests that stay
		// valid past the window.
		{"future-dated beyond the window", strconv.FormatInt(now.Add(6*time.Minute).Unix(), 10), http.StatusUnauthorized},
		{"absent", "", http.StatusUnauthorized},
		{"unparseable", "not-a-number", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reached := 0
			h := Webhook(freshSpec(now, 5*time.Minute, &reached), quietLog())
			if got := postAt(h, tc.stamp); got != tc.want {
				t.Fatalf("answered %d, want %d", got, tc.want)
			}
			if tc.want != http.StatusOK && reached != 0 {
				t.Fatal("a refused request still reached the handler")
			}
		})
	}
}

// An edge with no freshness declared is unbounded, which is the state every
// chassis edge was in before the stage.
func TestChassisWithNoFreshnessAdmitsAnyTimestamp(t *testing.T) {
	reached := 0
	h := Webhook(meteredSpec(nil, &reached), quietLog())
	if got := postAt(h, "1"); got != http.StatusOK {
		t.Fatalf("an edge with no freshness bound refused an ancient timestamp: %d", got)
	}
}

// The HubSpot receiver does not ride the chassis and parses milliseconds, so
// the comparison is what the two share. Held here because a one-directional
// mistake in either copy is invisible from the other.
func TestWithinSkewIsAbsoluteAndInclusive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"same instant", now, true},
		{"past, inside", now.Add(-time.Minute), true},
		{"future, inside", now.Add(time.Minute), true},
		{"past, exactly at the bound", now.Add(-5 * time.Minute), true},
		{"future, exactly at the bound", now.Add(5 * time.Minute), true},
		{"past, outside", now.Add(-5*time.Minute - time.Second), false},
		{"future, outside", now.Add(5*time.Minute + time.Second), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinSkew(now, tc.at, 5*time.Minute); got != tc.want {
				t.Fatalf("withinSkew = %v, want %v", got, tc.want)
			}
		})
	}
}
