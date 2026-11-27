// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestPrefersAbsentReadsThePreferenceAsRFC7240Writes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		headers []string
		want    bool
	}{
		{"alone", []string{"absent=204"}, true},
		{"token case", []string{"ABSENT=204"}, true},
		{"quoted value", []string{`absent="204"`}, true},
		{"among others", []string{"respond-async, wait=10, absent=204"}, true},
		{"with a parameter", []string{"absent=204; foo=bar"}, true},
		{"in a second header line", []string{"return=minimal", " absent = 204 "}, true},
		{"no header", nil, false},
		{"another value", []string{"absent=404"}, false},
		{"another preference only", []string{"return=minimal"}, false},
		{"inside a quoted value", []string{`note="absent=204, x"`}, false},
		{"a longer token", []string{"absently=204"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := PrefersAbsent(c.headers); got != c.want {
				t.Errorf("PrefersAbsent(%q) = %v, want %v", c.headers, got, c.want)
			}
		})
	}
}

// served runs err through Write behind the middleware, for a route that does
// or does not declare the preference.
func served(declared bool, prefer string, err error) *httptest.ResponseRecorder {
	handler := AbsentPreferenceFor(func(*http.Request) bool { return declared })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { Write(w, r, err) }))
	req := httptest.NewRequest(http.MethodGet, "/v1/brief", nil)
	if prefer != "" {
		req.Header.Set("Prefer", prefer)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestNothingYetAnswers204OnlyToARequestThatPrefersIt(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("reading the brief: %w", apperrors.ErrNothingYet)

	rec := served(true, "absent=204", wrapped)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("preferred nothing yet: got %d %q, want 204 and no body", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Preference-Applied"); got != AbsentPreference {
		t.Errorf("Preference-Applied = %q, want %q", got, AbsentPreference)
	}
	if got := rec.Header().Get("Vary"); got != "Prefer" {
		t.Errorf("Vary = %q, want Prefer", got)
	}

	rec = served(true, "", wrapped)
	if rec.Code != http.StatusNotFound || rec.Header().Get("Preference-Applied") != "" {
		t.Fatalf("nothing yet without the preference: got %d, want a plain 404", rec.Code)
	}
	if got := rec.Header().Get("Vary"); got != "Prefer" {
		t.Errorf("a 404 of a declaring operation: Vary = %q, want Prefer", got)
	}
}

func TestARowScopeMissStays404WithThePreference(t *testing.T) {
	t.Parallel()
	rec := served(true, "absent=204", apperrors.ErrNotFound)
	if rec.Code != http.StatusNotFound || rec.Header().Get("Preference-Applied") != "" {
		t.Fatalf("hidden row with the preference: got %d, want 404 without Preference-Applied", rec.Code)
	}
}

func TestAnOperationThatDoesNotDeclareThePreferenceIgnoresIt(t *testing.T) {
	t.Parallel()
	rec := served(false, "absent=204", apperrors.ErrNothingYet)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("undeclared operation: got %d, want 404", rec.Code)
	}
	if rec.Header().Get("Vary") != "" || rec.Header().Get("Preference-Applied") != "" {
		t.Errorf("undeclared operation: Vary %q, Preference-Applied %q, want neither",
			rec.Header().Get("Vary"), rec.Header().Get("Preference-Applied"))
	}
}

func TestNothingYetIsNotFoundToEveryOtherReader(t *testing.T) {
	t.Parallel()
	if !errors.Is(apperrors.ErrNothingYet, apperrors.ErrNotFound) {
		t.Fatal("ErrNothingYet must still be ErrNotFound, or every reader that is not a preferring request changes")
	}
}
