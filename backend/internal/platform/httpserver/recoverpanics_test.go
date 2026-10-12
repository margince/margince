// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A handler that aborts a body it has begun streaming must leave the client
// with a failed transfer. A recovered abort would end the chunked body cleanly,
// and the client would read the cut-off bytes as the whole download.
func TestAnAbortedStreamReachesTheClientAsAFailedTransfer(t *testing.T) {
	for name, abort := range map[string]error{
		"the abort itself": http.ErrAbortHandler,
		"a wrapped abort":  fmt.Errorf("export bundle: %w", http.ErrAbortHandler),
	} {
		t.Run(name, func(t *testing.T) {
			streamed := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/zip")
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte("PK partial archive")); err != nil {
					t.Errorf("writing the first chunk: %v", err)
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					t.Errorf("flushing the first chunk: %v", err)
				}
				panic(abort)
			})
			if _, readErr := fetch(t, RecoverPanics(slog.New(slog.DiscardHandler), streamed)); readErr == nil {
				t.Fatal("the cut-off stream read as a complete body")
			}
		})
	}
}

// A panic value that cannot be compared still answers the opaque 500.
func TestAPanicWithAnUncomparableValueAnswersFiveHundred(t *testing.T) {
	broken := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic([]string{"not", "comparable"}) })
	if status, _ := fetch(t, RecoverPanics(slog.New(slog.DiscardHandler), broken)); status != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", status)
	}
}

// fetch serves handler, GETs it once and reads the body to the end. The read
// error is the result: a cut-off stream shows only there.
func fetch(t *testing.T, handler http.Handler) (status int, readErr error) {
	t.Helper()
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("closing the body: %v", cerr)
		}
	}()
	_, readErr = io.ReadAll(resp.Body)
	return resp.StatusCode, readErr
}
