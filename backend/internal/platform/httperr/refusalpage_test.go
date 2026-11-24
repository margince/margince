// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

const browserAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

func TestOnlyABrowserNavigationPrefersHTML(t *testing.T) {
	for accept, want := range map[string]bool{
		browserAccept:      true,
		"text/html":        true,
		"":                 false,
		"*/*":              false,
		"application/json": false,
		"application/problem+json, text/html;q=0.5": false,
		"text/html;q=0":               false,
		"text/html;q=abc":             false,
		"text/html, application/json": true,
	} {
		if got := prefersHTML(accept); got != want {
			t.Errorf("prefersHTML(%q) = %v, want %v", accept, got, want)
		}
	}
}

func refusedWith(err error, accept string, page bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/attachments/x", nil)
	req.Header.Set("Accept", accept)
	if page {
		req = req.WithContext(WithRefusalPage(req.Context(), RefusalPage{
			Lang: "en", Title: "File not available", Sentence: "This file was deleted <or> moved.",
		}))
	}
	rec := httptest.NewRecorder()
	Write(rec, req, err)
	return rec
}

func TestARefusedFileOpenedInATabReadsAPage(t *testing.T) {
	for _, err := range []error{apperrors.ErrNotFound, apperrors.ErrPermissionDenied} {
		rec := refusedWith(err, browserAccept, true)
		if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("%v: content type %q, want a page", err, rec.Header().Get("Content-Type"))
		}
		if want, _ := Classify(err); rec.Code != want.Status {
			t.Errorf("%v: status %d, want %d", err, rec.Code, want.Status)
		}
		if !strings.Contains(rec.Body.String(), "This file was deleted &lt;or&gt; moved.") {
			t.Errorf("%v: the page does not carry the escaped sentence:\n%s", err, rec.Body.String())
		}
	}
}

func TestEveryOtherRefusalStaysAProblem(t *testing.T) {
	cases := map[string]*httptest.ResponseRecorder{
		"an API client":            refusedWith(apperrors.ErrNotFound, "application/json", true),
		"a script's default":       refusedWith(apperrors.ErrNotFound, "*/*", true),
		"a route with no page":     refusedWith(apperrors.ErrNotFound, browserAccept, false),
		"a refusal other than 404": refusedWith(apperrors.ErrConflict, browserAccept, true),
	}
	for name, rec := range cases {
		if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Errorf("%s: content type %q, want application/problem+json", name, got)
		}
	}
}
