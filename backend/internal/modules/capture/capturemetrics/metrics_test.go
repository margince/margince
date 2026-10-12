// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturemetrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// fresh swaps the process collector for an empty one for the length of a
// test, so the context observers can be asserted without another test's
// samples.
func fresh(t *testing.T) {
	t.Helper()
	previous := shared
	shared = newCollector()
	t.Cleanup(func() { shared = previous })
}

func render() string {
	var b strings.Builder
	WriteProcessMetrics(&b)
	return b.String()
}

// mustContain matches each want as a whole line, so `} 1` cannot be satisfied
// by `} 10`.
func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing from the exposition:\n\t%s\ngot:\n%s", want, out)
		}
	}
}

func TestEveryFamilyIsDeclaredBeforeItsFirstSample(t *testing.T) {
	fresh(t)
	mustContain(t, render(),
		"# TYPE margince_connector_requests_total counter",
		"# TYPE margince_connector_rate_limited_total counter",
		"# TYPE margince_connector_request_duration_seconds histogram",
		"# TYPE margince_capture_backfill_messages_total counter",
		"# TYPE margince_capture_backfill_stage_seconds histogram",
		"# TYPE margince_capture_backfill_pages_total counter",
		"# TYPE margince_capture_backfill_snooze_seconds_total counter",
		"# TYPE margince_capture_backfill_retry_after_seconds_total counter",
	)
}

func TestARequestIsCountedByItsResultAndTimedByItsOp(t *testing.T) {
	fresh(t)
	ObserveRequest("gmail", OpGetRaw, http.StatusOK, nil, 300*time.Millisecond)
	ObserveRequest("gmail", OpGetRaw, http.StatusTooManyRequests, &connector.RateLimitedError{}, time.Second)

	mustContain(t, render(),
		`margince_connector_requests_total{provider="gmail",op="get_raw",result="ok"} 1`,
		`margince_connector_requests_total{provider="gmail",op="get_raw",result="rate_limited"} 1`,
		`margince_connector_rate_limited_total{provider="gmail",op="get_raw",reason="unspecified"} 1`,
		`margince_connector_request_duration_seconds_bucket{provider="gmail",op="get_raw",le="0.25"} 0`,
		`margince_connector_request_duration_seconds_bucket{provider="gmail",op="get_raw",le="0.5"} 1`,
		`margince_connector_request_duration_seconds_bucket{provider="gmail",op="get_raw",le="+Inf"} 2`,
		`margince_connector_request_duration_seconds_sum{provider="gmail",op="get_raw"} 1.3`,
		`margince_connector_request_duration_seconds_count{provider="gmail",op="get_raw"} 2`,
	)
}

func TestARateLimitIsCountedUnderTheLimitItNamed(t *testing.T) {
	fresh(t)
	ObserveRequest("gmail", OpList, http.StatusForbidden,
		fmt.Errorf("list: %w", &connector.RateLimitedError{Reason: "userRateLimitExceeded", Status: http.StatusForbidden}), time.Second)
	ObserveRequest("gmail", OpList, http.StatusTooManyRequests,
		&connector.RateLimitedError{Reason: "notInTheSet"}, time.Second)
	ObserveRequest("gmail", OpList, http.StatusTooManyRequests, &connector.RateLimitedError{}, time.Second)
	ObserveRequest("gmail", OpList, http.StatusOK, nil, time.Second)

	out := render()
	mustContain(t, out,
		`margince_connector_rate_limited_total{provider="gmail",op="list",reason="userRateLimitExceeded"} 1`,
		`margince_connector_rate_limited_total{provider="gmail",op="list",reason="other"} 1`,
		`margince_connector_rate_limited_total{provider="gmail",op="list",reason="unspecified"} 1`,
	)
	if strings.Contains(out, `rate_limited_total{provider="gmail",op="list",reason="ok"`) {
		t.Errorf("a successful call was counted as rate limited:\n%s", out)
	}
}

func TestARequestResultNamesWhatARetryWouldChange(t *testing.T) {
	providerError := func(class error) error {
		return &connector.ProviderError{Op: "/messages", Status: http.StatusBadGateway, Class: class}
	}
	for _, tc := range []struct {
		name   string
		status int
		err    error
		want   string
	}{
		{"a 200", http.StatusOK, nil, resultOK},
		{"a rate limit", http.StatusForbidden, fmt.Errorf("page: %w", &connector.RateLimitedError{}), resultRateLimited},
		{"a refused credential", http.StatusUnauthorized, providerError(connector.ErrAuthRejected), resultAuth},
		{"a vanished message", http.StatusNotFound, providerError(connector.ErrUnreachable), resultNotFound},
		{"an outage", http.StatusBadGateway, providerError(connector.ErrUnreachable), resultUnreachable},
		{"a request never built", 0, errors.New("building request"), resultError},
	} {
		if got := requestResult(tc.status, tc.err); got != tc.want {
			t.Errorf("%s: result = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAPageIsClassedTheWayThePagerDecidesItsFate(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, resultOK},
		{&connector.RateLimitedError{RetryAfter: time.Minute}, resultRateLimited},
		{fmt.Errorf("gmail: %w", connector.ErrUnreachable), resultUnreachable},
		{connector.ErrAuthRejected, resultFailed},
		{errors.Join(connector.ErrUnreachable, connector.ErrAuthRejected), resultFailed},
		{connector.ErrCursorGone, resultTokenGone},
	} {
		if got := pageResult(tc.err); got != tc.want {
			t.Errorf("pageResult(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// The observers that read a run off the context record nothing outside one:
// the sink serves incremental sync too, and its stages are not a backfill's.
func TestNothingIsRecordedOutsideARun(t *testing.T) {
	fresh(t)
	ctx := context.Background()
	ObserveStage(ctx, StageSink, time.Second)
	ObservePage(ctx, nil)
	ObservePacing(ctx, time.Second)
	ObserveDeferral(ctx, time.Minute, connector.ErrUnreachable)
	ObserveResumed(ctx, time.Second)
	ObserveInPageWait(ctx, time.Second, time.Second)
	ctx, message := BeginMessage(ctx)
	NoteOutcome(ctx, "internal")
	message.End(true, nil)
	message.Refuse()

	ObservePacing(WithRun(context.Background()), time.Second)

	if out := render(); strings.Contains(out, "{") {
		t.Errorf("a context with no named provider recorded samples:\n%s", out)
	}
}

func TestTheJobsRunLearnsItsProviderFromThePager(t *testing.T) {
	fresh(t)
	jobCtx := WithRun(context.Background())
	pageCtx := ForProvider(jobCtx, "gmail")
	ObservePage(pageCtx, nil)
	ObserveStage(pageCtx, StageFetch, 2*time.Second)
	ObservePacing(jobCtx, time.Second)

	mustContain(t, render(),
		`margince_capture_backfill_pages_total{provider="gmail",result="ok"} 1`,
		`margince_capture_backfill_stage_seconds_count{provider="gmail",stage="fetch"} 1`,
		`margince_capture_backfill_snooze_seconds_total{provider="gmail",reason="pacing"} 1`,
	)
}

// The gap between what the backfill waited and what the provider asked for is
// what says whether the ladder or Google is setting the pace.
func TestAFaultsWaitIsCountedBesideTheRetryAfterTheProviderAsked(t *testing.T) {
	fresh(t)
	ctx := ForProvider(context.Background(), "gmail")
	ObserveDeferral(ctx, 240*time.Second, fmt.Errorf("page: %w", &connector.RateLimitedError{RetryAfter: 30 * time.Second}))
	ObserveDeferral(ctx, 60*time.Second, fmt.Errorf("page: %w", connector.ErrRateLimited))
	ObserveDeferral(ctx, 20*time.Second, fmt.Errorf("page: %w", connector.ErrUnreachable))
	ObserveDeferral(ctx, time.Second, fmt.Errorf("page: %w", connector.ErrCursorGone))
	ObserveDeferral(ctx, 10*time.Second, errors.New("a run of refused messages"))
	ObserveInPageWait(ctx, 5*time.Second, 4*time.Second)
	ObserveResumed(ctx, time.Second)

	mustContain(t, render(),
		`margince_capture_backfill_snooze_seconds_total{provider="gmail",reason="rate_limited"} 300`,
		`margince_capture_backfill_snooze_seconds_total{provider="gmail",reason="unreachable"} 20`,
		`margince_capture_backfill_snooze_seconds_total{provider="gmail",reason="token_rejected"} 1`,
		`margince_capture_backfill_snooze_seconds_total{provider="gmail",reason="internal"} 10`,
		`margince_capture_backfill_snooze_seconds_total{provider="gmail",reason="rate_limited_in_page"} 5`,
		`margince_capture_backfill_snooze_seconds_total{provider="gmail",reason="resumed"} 1`,
		`margince_capture_backfill_retry_after_seconds_total{provider="gmail"} 34`,
	)
}

func TestAMessageEndsOnItsTracedDecisionOrOnHowItsWalkEnded(t *testing.T) {
	fresh(t)
	run := ForProvider(context.Background(), "gmail")
	walk := func(traced string, captured bool, err error) {
		ctx, message := BeginMessage(run)
		if traced != "" {
			NoteOutcome(ctx, traced)
		}
		message.End(captured, err)
	}
	walk("internal", false, nil)
	walk("deferred", true, nil)
	walk("", true, nil)
	walk("", false, nil)
	walk("captured", true, connector.ErrUnreachable)
	_, refused := BeginMessage(run)
	refused.Refuse()

	mustContain(t, render(),
		`margince_capture_backfill_messages_total{provider="gmail",outcome="internal"} 1`,
		`margince_capture_backfill_messages_total{provider="gmail",outcome="deferred"} 1`,
		`margince_capture_backfill_messages_total{provider="gmail",outcome="captured"} 1`,
		`margince_capture_backfill_messages_total{provider="gmail",outcome="skipped"} 1`,
		`margince_capture_backfill_messages_total{provider="gmail",outcome="failed"} 1`,
		`margince_capture_backfill_messages_total{provider="gmail",outcome="refused"} 1`,
	)
}

// A provider name reaches the exposition through the escaper, never raw.
func TestALabelValueIsEscapedForTheTextFormat(t *testing.T) {
	fresh(t)
	ObserveRequest("odd\"name", OpOther, http.StatusOK, nil, time.Millisecond)
	mustContain(t, render(), `margince_connector_requests_total{provider="odd\"name",op="other",result="ok"} 1`)
}
