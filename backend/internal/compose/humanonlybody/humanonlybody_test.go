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
		if got := Refused(w, r, Marks{HumanOnly: members}, 32); got != refused {
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
	if Refused(httptest.NewRecorder(), httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"kind":1}`)), Marks{}, 32) {
		t.Error("a route with no human-only member refused a body")
	}
}

// An agent sending a member a value outside the ones it may send is refused.
func TestAValueTheAgentMayNotSendIsRefused(t *testing.T) {
	marks := Marks{AgentValues: map[string][]string{"source": {"agent"}}}
	for body, refused := range map[string]bool{
		`{"timezone":"Europe/Lisbon","source":"agent"}`:   false,
		`{"timezone":"Europe/Lisbon","source":"chosen"}`:  true,
		`{"timezone":"Europe/Lisbon","Source":"browser"}`: true,
		`{"timezone":"Europe/Lisbon","source":1}`:         true,
		`{"timezone":"Europe/Lisbon"}`:                    false,
	} {
		r := httptest.NewRequest(http.MethodPut, "/v1/me/timezone", strings.NewReader(body))
		w := httptest.NewRecorder()
		if got := Refused(w, r, marks, 256); got != refused {
			t.Errorf("%s: refused = %v, want %v", body, got, refused)
			continue
		}
		if refused && (w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), codeAgentValue) ||
			!strings.Contains(w.Body.String(), "call again with source agent")) {
			t.Errorf("%s: answered %d %s, want 422 naming the member and the value to send", body, w.Code, w.Body)
		}
	}
}
