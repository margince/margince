// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTheThankYouPageRendersItsMessageAsText(t *testing.T) {
	h := Handlers{store: &Store{log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	rec := httptest.NewRecorder()
	h.writePage(rec, httptest.NewRequest(http.MethodPost, "/", nil), http.StatusOK, `<script>alert(1)</script>`)
	body := rec.Body.String()
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("the message reached the page as markup:\n%s", body)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("Content-Security-Policy = %q, want scripts refused", csp)
	}
}

func TestOnlyAPlainFormPostIsAnsweredWithAPage(t *testing.T) {
	cases := []struct {
		accept, contentType string
		page                bool
	}{
		{"text/html,application/xhtml+xml", "application/x-www-form-urlencoded", true},
		{"*/*", "application/x-www-form-urlencoded", false},
		{"text/html", "application/json", false},
		{"application/json", "multipart/form-data; boundary=x", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Header.Set("Accept", c.accept)
		r.Header.Set("Content-Type", c.contentType)
		if got := wantsPage(r); got != c.page {
			t.Errorf("wantsPage(Accept %q, Content-Type %q) = %v, want %v", c.accept, c.contentType, got, c.page)
		}
	}
}

func TestEachRefusalHasItsOwnStatus(t *testing.T) {
	cases := map[error]int{
		errUnverified:             http.StatusUnauthorized,
		errForeignOrigin:          http.StatusForbidden,
		ErrPaused:                 http.StatusGone,
		errUnreadable:             http.StatusUnprocessableEntity,
		badInput("email", "need"): http.StatusUnprocessableEntity,
	}
	for err, want := range cases {
		rec := httptest.NewRecorder()
		writeErr(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
		if rec.Code != want {
			t.Errorf("%v → %d, want %d", err, rec.Code, want)
		}
	}
}
