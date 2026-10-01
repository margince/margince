// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
)

// sample reads one series' current value off the process exposition. The
// collector is process-wide, so every assertion here is a delta across the
// call under test.
func sample(t *testing.T, series string) float64 {
	t.Helper()
	var b strings.Builder
	capturemetrics.WriteProcessMetrics(&b)
	for _, line := range strings.Split(b.String(), "\n") {
		if value, ok := strings.CutPrefix(line, series+" "); ok {
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				t.Fatalf("series %s carries an unparseable value %q", series, value)
			}
			return f
		}
	}
	return 0
}

// deltas captures each series before act runs and answers how far it moved.
func deltas(t *testing.T, act func(), series ...string) map[string]float64 {
	t.Helper()
	before := map[string]float64{}
	for _, s := range series {
		before[s] = sample(t, s)
	}
	act()
	moved := map[string]float64{}
	for _, s := range series {
		moved[s] = sample(t, s) - before[s]
	}
	return moved
}

func requests(op, result string) string {
	return `margince_connector_requests_total{provider="gmail",op="` + op + `",result="` + result + `"}`
}

func TestAGetPathNamesTheCallItMakes(t *testing.T) {
	for path, want := range map[string]string{
		"/messages":    capturemetrics.OpList,
		"/messages/m1": capturemetrics.OpGetRaw,
		"/history":     capturemetrics.OpHistory,
		"/profile":     capturemetrics.OpOther,
		"/labels":      capturemetrics.OpOther,
	} {
		if got := getOp(path); got != want {
			t.Errorf("getOp(%q) = %q, want %q", path, got, want)
		}
	}
}

// Every call that reaches Google is counted under the result a retry would or
// would not change, the POSTs and the watch included.
func TestEveryGmailCallIsCountedByOpAndResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/messages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"messages": []map[string]string{{"id": "m1"}}})
	})
	mux.HandleFunc("/messages/gone", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/history", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	mux.HandleFunc("/profile", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	mux.HandleFunc("/messages/send", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	mux.HandleFunc("/watch", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"historyId": "9", "expiration": "1700000000000"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	api := NewAPI(srv.Client(), srv.URL)
	ctx := context.Background()

	moved := deltas(t, func() {
		if _, err := api.ListRecent(ctx, "access", 10); err != nil {
			t.Errorf("ListRecent: %v", err)
		}
		if _, err := api.GetRaw(ctx, "access", "gone"); !errors.Is(err, ErrMessageGone) {
			t.Errorf("GetRaw of a deleted message = %v, want ErrMessageGone", err)
		}
		if _, _, _, err := api.History(ctx, "access", "1"); err == nil {
			t.Error("a throttled history call succeeded")
		}
		if _, _, err := api.Profile(ctx, "access"); err == nil {
			t.Error("a refused profile call succeeded")
		}
		if _, err := api.Send(ctx, "access", "raw"); err == nil {
			t.Error("a failed send succeeded")
		}
		if _, _, err := api.Watch(ctx, "access", "topic"); err != nil {
			t.Errorf("Watch: %v", err)
		}
	},
		requests("list", "ok"), requests("get_raw", "not_found"), requests("history", "rate_limited"),
		requests("other", "auth"), requests("other", "unreachable"), requests("other", "ok"),
		`margince_connector_request_duration_seconds_count{provider="gmail",op="other"}`,
	)

	for series, want := range map[string]float64{
		requests("list", "ok"): 1, requests("get_raw", "not_found"): 1, requests("history", "rate_limited"): 1,
		requests("other", "auth"): 1, requests("other", "unreachable"): 1, requests("other", "ok"): 1,
		`margince_connector_request_duration_seconds_count{provider="gmail",op="other"}`: 3,
	} {
		if moved[series] != want {
			t.Errorf("%s moved by %v, want %v", series, moved[series], want)
		}
	}
}

func messages(outcome string) string {
	return `margince_capture_backfill_messages_total{provider="gmail",outcome="` + outcome + `"}`
}

func stageCount(stage string) string {
	return `margince_capture_backfill_stage_seconds_count{provider="gmail",stage="` + stage + `"}`
}

// A backfill page tallies every message it walks and times the two stages
// this connector owns: the download, and the parse of what came down.
func TestABackfillPageTalliesEachMessageAndTimesItsStages(t *testing.T) {
	api := &pagedAPI{pages: map[string][]string{"": {"m1@mail.gmail.com", "m2@mail.gmail.com", "m3@mail.gmail.com"}}}
	api.raws = map[string][]byte{
		"m1@mail.gmail.com": rawMsg("m1@mail.gmail.com", "alice@acme.com"),
		"m2@mail.gmail.com": []byte("not an rfc822 message"),
	}
	api.gone = map[string]bool{"m3@mail.gmail.com": true}
	c := New(fakeOAuth{access: "access-1"}, api)
	ctx := capturemetrics.ForProvider(context.Background(), connectorName)

	moved := deltas(t, func() {
		if _, err := c.BackfillPage(ctx, authBytes(t), time.Now(), "", &recordingSink{}); err != nil {
			t.Errorf("page: %v", err)
		}
	}, messages("captured"), messages("skipped"), stageCount("fetch"), stageCount("parse"))

	for series, want := range map[string]float64{
		messages("captured"): 1, messages("skipped"): 2, stageCount("fetch"): 3, stageCount("parse"): 2,
	} {
		if moved[series] != want {
			t.Errorf("%s moved by %v, want %v", series, moved[series], want)
		}
	}
}

func TestAMessageThatStopsThePageIsCountedFailed(t *testing.T) {
	api := &pagedAPI{pages: map[string][]string{"": {"m1@mail.gmail.com"}}}
	api.raws = map[string][]byte{"m1@mail.gmail.com": rawMsg("m1@mail.gmail.com", "alice@acme.com")}
	c := New(fakeOAuth{access: "access-1"}, api)
	ctx := capturemetrics.ForProvider(context.Background(), connectorName)

	moved := deltas(t, func() {
		if _, err := c.BackfillPage(ctx, authBytes(t), time.Now(), "", failingSink{err: errors.New("db down")}); err == nil {
			t.Error("a sink fault did not stop the page")
		}
	}, messages("failed"))
	if moved[messages("failed")] != 1 {
		t.Errorf("failed moved by %v, want 1", moved[messages("failed")])
	}
}

// Every refresh is a round trip to Google's token endpoint, so a page that
// starts with one shows it in the request count and its latency.
func TestATokenRefreshIsCountedAsAProviderCall(t *testing.T) {
	ctx := context.Background()
	moved := deltas(t, func() {
		if _, err := New(fakeOAuth{access: "access-1"}, &pagedAPI{}).EstimateBackfill(ctx, authBytes(t), time.Now()); err != nil {
			t.Errorf("EstimateBackfill: %v", err)
		}
		if _, err := New(staleOAuth{}, &pagedAPI{}).EstimateBackfill(ctx, authBytes(t), time.Now()); err == nil {
			t.Error("a revoked refresh token was accepted")
		}
		if _, err := (timedAuthorizer{staleOAuth{}}).Exchange(ctx, "code", "https://back"); err == nil {
			t.Error("a refused exchange succeeded")
		}
	}, requests("token", "ok"), requests("token", "error"))
	if moved[requests("token", "ok")] != 1 || moved[requests("token", "error")] != 2 {
		t.Errorf("token calls moved ok by %v and error by %v, want 1 and 2",
			moved[requests("token", "ok")], moved[requests("token", "error")])
	}
}
