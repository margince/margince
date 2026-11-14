// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestATaggedAnswerTheClientHoldsComesBackWithNoBody(t *testing.T) {
	serve := func(body any, ifNoneMatch string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/v1/worklist", nil)
		if ifNoneMatch != "" {
			r.Header.Set("If-None-Match", ifNoneMatch)
		}
		w := httptest.NewRecorder()
		WriteJSONTagged(w, r, body)
		return w
	}
	first := serve(map[string]int{"total": 3}, "")
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" || first.Body.Len() == 0 {
		t.Fatalf("a first read answers %d with tag %q and %d bytes, want 200, a tag and the body", first.Code, etag, first.Body.Len())
	}
	if again := serve(map[string]int{"total": 3}, etag); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Errorf("an unchanged answer the client holds comes back %d with %d bytes, want 304 and none", again.Code, again.Body.Len())
	}
	for _, w := range []*httptest.ResponseRecorder{first, serve(map[string]int{"total": 3}, etag)} {
		if got := w.Header().Get("Cache-Control"); got != "private, no-cache" {
			t.Errorf("answer %d carries Cache-Control %q, want private, no-cache so no shared cache keeps it", w.Code, got)
		}
	}
	if strong := serve(map[string]int{"total": 3}, strings.TrimPrefix(etag, "W/")); strong.Code != http.StatusNotModified {
		t.Errorf("the tag without its weak prefix comes back %d, want 304", strong.Code)
	}
	if !strings.HasPrefix(etag, `W/"`) {
		t.Errorf("the tag %q is strong, but gzip changes the bytes it names", etag)
	}
	if changed := serve(map[string]int{"total": 4}, etag); changed.Code != http.StatusOK || changed.Header().Get("ETag") == etag {
		t.Errorf("a changed answer comes back %d with tag %q, want 200 and a new tag", changed.Code, changed.Header().Get("ETag"))
	}
}
