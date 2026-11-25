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
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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

func plainPost(lang textlang.Lang) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r.WithContext(systemtext.WithLocale(r.Context(), lang))
}

func TestARefusedPlainPostReadsInTheVisitorsLanguage(t *testing.T) {
	h := Handlers{store: &Store{log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	cases := []struct {
		name   string
		err    error
		lang   textlang.Lang
		status int
		want   string
	}{
		{"consent in German", badInput(fieldConsent, "this form needs the consent box ticked"), textlang.German, http.StatusUnprocessableEntity, "da die Einwilligung nicht angekreuzt ist"},
		{"consent in Vietnamese", badInput(fieldConsent, "this form needs the consent box ticked"), textlang.Vietnamese, http.StatusUnprocessableEntity, "ô đồng ý chưa được đánh dấu"},
		{"email in English", badInput(fieldEmail, "is not an email address"), textlang.English, http.StatusUnprocessableEntity, "the email address is missing or not valid"},
		{"archived form in German", apperrors.ErrNotFound, textlang.German, http.StatusNotFound, "Dieses Formular ist nicht mehr verfügbar."},
		{"paused form in Vietnamese", ErrPaused, textlang.Vietnamese, http.StatusGone, "Biểu mẫu này hiện không nhận lượt gửi mới."},
		{"throttled in German", ThrottleVerdict{Refused: true, RetryAfter: 30 * time.Second}.Refusal(), textlang.German, http.StatusTooManyRequests, "Bitte 30 Sekunden warten"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.AnswerRefusal(rec, plainPost(c.lang), c.err)
		body := rec.Body.String()
		if rec.Code != c.status || !strings.Contains(body, c.want) {
			t.Errorf("%s: status %d, want %d; page lacks %q:\n%s", c.name, rec.Code, c.status, c.want, body)
		}
		if !strings.Contains(body, `<html lang="`+string(c.lang)+`">`) {
			t.Errorf("%s: the page does not declare its language %q:\n%s", c.name, c.lang, body)
		}
		if strings.Contains(body, "Please go back and try again") || strings.Contains(body, "consent box ticked.") {
			t.Errorf("%s: the page shows the English reason a script reads:\n%s", c.name, body)
		}
	}
}

func TestAThrottledPlainPostKeepsItsRetryAfter(t *testing.T) {
	h := Handlers{store: &Store{log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	rec := httptest.NewRecorder()
	h.AnswerRefusal(rec, plainPost(textlang.English), ThrottleVerdict{Refused: true, RetryAfter: 1500 * time.Millisecond}.Refusal())
	if got := rec.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
}

func TestAScriptPostingToAnArchivedFormStillReadsTheProblem(t *testing.T) {
	h := Handlers{store: &Store{log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	r := plainPost(textlang.German)
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.AnswerRefusal(rec, r, apperrors.ErrNotFound)
	if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("status %d, Content-Type %q, want 404 problem JSON", rec.Code, rec.Header().Get("Content-Type"))
	}
}
