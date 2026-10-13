// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

// The server's own deadline ends the request context with DeadlineExceeded:
// that is its failure to finish, not a caller leaving.
func TestARequestThatTimedOutIsStillAServerFault(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	req := httptest.NewRequest(http.MethodGet, "/v1/worklist", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	Write(rec, req, errors.New("conn closed"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("a timed-out request was answered %d, want 500", rec.Code)
	}
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
