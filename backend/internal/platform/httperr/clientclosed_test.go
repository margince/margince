// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A caller who leaves closes the connection under the work. A later read on it
// fails with an error that carries no context error. The request's own
// cancellation is what says the caller left.
func TestACallerWhoLeftIsNotAServerFaultWhateverTheErrorSays(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/worklist", nil).WithContext(ctx)
	cancel()
	rec := httptest.NewRecorder()

	Write(rec, req, errors.New("reading worklist pins: failed to deallocate cached statement(s): conn closed"))

	if rec.Code != statusClientClosedRequest {
		t.Errorf("a request whose caller left was answered %d, want %d", rec.Code, statusClientClosedRequest)
	}
	if logged.Len() != 0 {
		t.Errorf("a request whose caller left logged at warn or above: %s", logged.String())
	}
}

// The line a caller-left request leaves keeps the error, so a real fault that
// happened beside the cancel can still be found.
func TestACallerLeftLineKeepsTheCause(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/worklist", nil).WithContext(ctx)
	cancel()

	Write(httptest.NewRecorder(), req, errors.New("conn closed"))

	if !bytes.Contains(logged.Bytes(), []byte("conn closed")) {
		t.Errorf("the caller-left line dropped the error: %s", logged.String())
	}
}

// The server's own deadline ends the request context with DeadlineExceeded. It
// is answered as a timeout the caller may retry, never as an unhandled 500.
func TestARequestPastItsDeadlineIsAnsweredAsARetryableTimeout(t *testing.T) {
	for name, cause := range map[string]error{
		"a driver error naming no context": errors.New("conn closed"),
		"a sentinel around the deadline":   fmt.Errorf("%w: %w", apperrors.ErrConflict, context.DeadlineExceeded),
	} {
		t.Run(name, func(t *testing.T) {
			var logged bytes.Buffer
			restore := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(restore) })
			rec := httptest.NewRecorder()

			Write(rec, pastDeadline(t, "/v1/contacts"), cause)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("a timed-out request was answered %d, want 503", rec.Code)
			}
			if got := problemCode(t, rec); got != requestTimedOutCode {
				t.Errorf("code = %q, want %q", got, requestTimedOutCode)
			}
			if got := rec.Header().Get("Retry-After"); got != "2" {
				t.Errorf("Retry-After = %q, want 2", got)
			}
			if !bytes.Contains(logged.Bytes(), []byte("time limit")) || bytes.Contains(logged.Bytes(), []byte("unhandled")) {
				t.Errorf("the log does not name the timeout: %s", logged.String())
			}
		})
	}
}

// A refusal the handler decided keeps its own answer when the deadline also
// passed: the deadline did not cause it.
func TestARefusalAfterTheDeadlineKeepsItsOwnAnswer(t *testing.T) {
	rec := httptest.NewRecorder()

	Write(rec, pastDeadline(t, "/v1/contacts/1"), apperrors.ErrNotFound)

	if rec.Code != http.StatusNotFound {
		t.Errorf("a not-found after the deadline was answered %d, want 404", rec.Code)
	}
}

func TestATimedOutRequestIsWorthRetrying(t *testing.T) {
	if !(Fault{Code: requestTimedOutCode}).Transient() {
		t.Error("a timed-out request is reported as settled, so an agent never sends it again")
	}
}

// pastDeadline is a request whose context passed its deadline before it was
// built, so no test waits on a clock.
func pastDeadline(t *testing.T, path string) *http.Request {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	t.Cleanup(cancel)
	return httptest.NewRequest(http.MethodPost, path, nil).WithContext(ctx)
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not a problem body: %v (%q)", err, rec.Body.String())
	}
	return body.Code
}

// A client that abandons a request is not a server fault: the store call
// returns the canceled context's error and nobody is left to read an answer, so
// the operator's error log and 5xx series must not count it.
func TestARequestTheClientCanceledIsNotAServerFault(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/companies/1/360", nil).WithContext(ctx)
	cancel()
	rec := httptest.NewRecorder()

	Write(rec, req, fmt.Errorf("read the company: %w", context.Canceled))

	if rec.Code != statusClientClosedRequest {
		t.Errorf("a canceled request was answered %d, want %d (client closed request, not a 5xx)", rec.Code, statusClientClosedRequest)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("a canceled request was answered with a body: %q", rec.Body.String())
	}
	if logged.Len() != 0 {
		t.Errorf("a canceled request logged at warn or above: %s", logged.String())
	}
}

// A deadline the server set is its own failure to finish in time, and a context
// error on a request whose caller is still there is not the caller leaving.
func TestAContextErrorWhileTheCallerIsStillThereStaysAFault(t *testing.T) {
	for name, cause := range map[string]error{
		"a server deadline":               context.DeadlineExceeded,
		"a cancel while the caller waits": context.Canceled,
	} {
		t.Run(name, func(t *testing.T) {
			var logged bytes.Buffer
			restore := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(restore) })

			rec := httptest.NewRecorder()
			Write(rec, httptest.NewRequest(http.MethodGet, "/v1/companies/1/360", nil), fmt.Errorf("read: %w", cause))

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("answered %d, want 500", rec.Code)
			}
			if logged.Len() == 0 {
				t.Error("a real fault was not logged")
			}
		})
	}
}
