// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package humanonlybody

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A body setting a human-only member is refused; any other body, and one too
// large or not an object, reaches the handler unchanged.
func TestABodySettingAHumanOnlyMemberIsRefused(t *testing.T) {
	members := []string{"evidence", "kind"}
	for body, refused := range map[string]bool{
		`{"kind":"risk"}`:           true,
		`{"Kind":"risk"}`:           true,
		`{"evidence":[]}`:           true,
		`{"status":"acknowledged"}`: false,
		`not json`:                  false,
		`{"kind":"` + strings.Repeat("x", 64) + `"}`: false,
	} {
		r := httptest.NewRequest(http.MethodPatch, "/v1/signals/x", strings.NewReader(body))
		w := httptest.NewRecorder()
		if got := Refused(w, r, members, 32); got != refused {
			t.Errorf("%.30s: refused = %v, want %v", body, got, refused)
			continue
		}
		if refused && w.Code != http.StatusForbidden {
			t.Errorf("%s: answered %d, want 403", body, w.Code)
		}
		rest, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !refused && string(rest) != body {
			t.Errorf("%.30s: the handler would read %q, want the body unchanged", body, rest)
		}
	}
	if Refused(httptest.NewRecorder(), httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"kind":1}`)), nil, 32) {
		t.Error("a route with no human-only member refused a body")
	}
}
