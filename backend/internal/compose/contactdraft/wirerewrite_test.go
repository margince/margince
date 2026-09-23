// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

// The wire mapping, asked of the decoder the handler actually uses.
//
// A mapping that dropped the draft on screen would look exactly like a caller
// who sent none — and a caller who sent none is a first draft, which is the
// behaviour a rewrite has to stop producing.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTheDraftOnScreenReachesTheRequest(t *testing.T) {
	const shown = "Guten Tag Frau Malherbe,\n\nwir liefern am Montag."

	req, ok := decodeRequest(httptest.NewRecorder(), postJSON(
		`{"intent":"kurz halten","rewrite_of":`+quote(shown)+`}`))
	if !ok {
		t.Fatal("a body carrying the shown draft was refused")
	}
	if req.RewriteOf != shown {
		t.Fatalf("RewriteOf = %q, want the draft the composer is showing", req.RewriteOf)
	}
	// The steering rides alongside rather than being replaced by it: the two
	// say different things, and a rewrite verb that arrived with no draft
	// would generate rather than revise.
	if req.Intent != "kurz halten" {
		t.Fatalf("Intent = %q, want the caller's own steering", req.Intent)
	}
}

func TestAFirstDraftCarriesNothingToRewrite(t *testing.T) {
	req, ok := decodeRequest(httptest.NewRecorder(), postJSON(`{"intent":"kurz halten"}`))
	if !ok {
		t.Fatal("a body with no rewrite_of was refused")
	}
	if req.RewriteOf != "" {
		t.Fatalf("an absent rewrite_of became %q — a first draft has nothing to rewrite", req.RewriteOf)
	}

	// And an ABSENT body at all, which is the ordinary "write to this contact"
	// call and the one shape that reaches the decoder's early return.
	empty, ok := decodeRequest(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil))
	if !ok || empty.RewriteOf != "" || empty.Intent != "" {
		t.Fatalf("an absent body decoded as %+v, want the zero request", empty)
	}
}

// postJSON builds the request the handler is handed, with a Content-Length the
// decoder reads: an absent body is its early return, so a fixture that left the
// length at zero would take that path for every case.
func postJSON(body string) *http.Request {
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	return req
}

// quote renders a Go string as a JSON one, so a fixture carrying a newline is
// the same text on both sides of the wire.
func quote(s string) string {
	out, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(out)
}
