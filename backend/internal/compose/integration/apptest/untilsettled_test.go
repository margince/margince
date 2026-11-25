// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// scripted answers each request with the next of answers, the last one for
// every request after it, and counts the requests.
func scripted(t *testing.T, answers ...func(http.ResponseWriter)) (*AppEnv, *atomic.Int32) {
	t.Helper()
	var sent atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(sent.Add(1)) - 1
		answers[min(n, len(answers)-1)](w)
	}))
	t.Cleanup(srv.Close)
	return &AppEnv{TS: srv, Client: srv.Client()}, &sent
}

func answer(t *testing.T, status int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("writing the scripted answer: %v", err)
		}
	}
}

func refusal(t *testing.T, status int, code string) func(http.ResponseWriter) {
	return answer(t, status, `{"code":"`+code+`"}`)
}

// A busy answer the server says clears on its own is sent again until the
// write lands, and the caller reads the final answer.
func TestATransientBusyAnswerIsSentAgainUntilItSettles(t *testing.T) {
	t.Parallel()
	e, sent := scripted(t, refusal(t, http.StatusServiceUnavailable, "write_contention"),
		refusal(t, http.StatusServiceUnavailable, "website_form_busy"), answer(t, http.StatusCreated, `{"id":"made"}`))
	var out struct {
		ID string `json:"id"`
	}
	if status := e.CallUntilSettled(t, "POST", "/v1/anything", nil, nil, &out); status != http.StatusCreated || out.ID != "made" {
		t.Fatalf("answered %d %+v, want the write that settled", status, out)
	}
	if sent.Load() != 3 {
		t.Errorf("sent %d requests, want 3", sent.Load())
	}
}

// Any other refusal is the answer at once, and a busy answer that never clears
// stops after a bounded number of tries.
func TestOnlyATransientBusyAnswerIsSentAgain(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		answer func(http.ResponseWriter)
		status int
		sends  int32
	}{
		"a 503 that is not transient": {refusal(t, http.StatusServiceUnavailable, "service_unavailable"), http.StatusServiceUnavailable, 1},
		"a transient code on a 429":   {refusal(t, http.StatusTooManyRequests, "rate_limited"), http.StatusTooManyRequests, 1},
		"a conflict":                  {refusal(t, http.StatusConflict, "conflict"), http.StatusConflict, 1},
		"busy for good":               {refusal(t, http.StatusServiceUnavailable, "write_contention"), http.StatusServiceUnavailable, settleTries},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e, sent := scripted(t, tc.answer)
			if status := e.CallUntilSettled(t, "POST", "/v1/anything", nil, nil, nil); status != tc.status || sent.Load() != tc.sends {
				t.Errorf("answered %d after %d requests, want %d after %d", status, sent.Load(), tc.status, tc.sends)
			}
		})
	}
}
