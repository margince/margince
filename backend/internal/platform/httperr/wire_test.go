// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeInto(t *testing.T, body string) (ok bool, status int) {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/things", strings.NewReader(body))
	var into map[string]any
	ok = Decode(w, r, &into)
	return ok, w.Code
}

func TestDecodeAcceptsExactlyOneJSONValue(t *testing.T) {
	if ok, _ := decodeInto(t, `{"a":1}`); !ok {
		t.Fatal("plain object refused")
	}
	// Trailing tokens are malformed, not silently ignored — two values
	// in one body is an ambiguous payload.
	if ok, status := decodeInto(t, `{"a":1}{"b":2}`); ok || status != 422 {
		t.Fatalf("trailing JSON accepted (ok=%v status=%d)", ok, status)
	}
	if ok, status := decodeInto(t, `{`); ok || status != 422 {
		t.Fatalf("malformed JSON accepted (ok=%v status=%d)", ok, status)
	}
}

func TestDecodeCapsTheBody(t *testing.T) {
	oversized := `{"pad":"` + strings.Repeat("x", MaxBodyBytes) + `"}`
	ok, status := decodeInto(t, oversized)
	if ok || status != 413 {
		t.Fatalf("oversized body → ok=%v status=%d, want refusal 413", ok, status)
	}
}

// RFC 9110 §13.1.2: If-None-Match is a comma-separated list, "*" matches
// anything, and comparison is WEAK — a proxy prefixing this server's own
// strong tag with "W/" must still count as a match, or a caller that adds
// one silently loses the conditional-GET saving the header exists to give it.
func TestIfNoneMatchHit(t *testing.T) {
	tests := map[string]struct {
		etag   string
		header string
		want   bool
	}{
		"no header":                {etag: `"abc123"`, header: "", want: false},
		"exact match":              {etag: `"abc123"`, header: `"abc123"`, want: true},
		"wildcard":                 {etag: `"abc123"`, header: "*", want: true},
		"weak-prefixed match":      {etag: `"abc123"`, header: `W/"abc123"`, want: true},
		"one of several, matches":  {etag: `"abc123"`, header: `"other", "abc123"`, want: true},
		"one of several, no match": {etag: `"abc123"`, header: `"other", "third"`, want: false},
		"different tag":            {etag: `"abc123"`, header: `"xyz789"`, want: false},
		"substring is not a match": {etag: `"abc123"`, header: `"abc1234"`, want: false},
		// A comma is a legal byte inside a quoted etag-value (RFC 9110's
		// etag-value grammar), so splitting the list on every comma would cut
		// this single candidate into two pieces, neither equal to the etag.
		"comma inside the etag value itself": {etag: `"a,b"`, header: `"a,b"`, want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/v1/things", nil)
			if tt.header != "" {
				r.Header.Set("If-None-Match", tt.header)
			}
			if got := IfNoneMatchHit(r, tt.etag); got != tt.want {
				t.Errorf("IfNoneMatchHit(If-None-Match: %q, etag: %q) = %v, want %v", tt.header, tt.etag, got, tt.want)
			}
		})
	}
}

func TestDownloadHeadersCarryTheRealNameOfANonASCIIFile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, filename, inline, want string }{
		{"plain ascii is unchanged", "report.pdf", "", `attachment; filename="report.pdf"`},
		{"a quote and a backslash are escaped", `a"b\c.txt`, "", `attachment; filename="a\"b\\c.txt"`},
		{"an umlaut", "Müller.pdf", "", `attachment; filename="M_ller.pdf"; filename*=UTF-8''M%C3%BCller.pdf`},
		{"accent, han and a space", "Zoë 李小龙.txt", "", `attachment; filename="Zo_ ___.txt"; filename*=UTF-8''Zo%C3%AB%20%E6%9D%8E%E5%B0%8F%E9%BE%99.txt`},
		{"a control character is dropped", "a\x00b.txt", "", `attachment; filename="ab.txt"; filename*=UTF-8''a%00b.txt`},
		{"inline", "Müller.pdf", "inline", `inline; filename="M_ller.pdf"; filename*=UTF-8''M%C3%BCller.pdf`},
	} {
		rec := httptest.NewRecorder()
		Download{Filename: tc.filename, Inline: tc.inline == "inline"}.WriteHeaders(rec)
		if got := rec.Header().Get("Content-Disposition"); got != tc.want {
			t.Errorf("%s: Content-Disposition = %q, want %q", tc.name, got, tc.want)
		}
	}
}
