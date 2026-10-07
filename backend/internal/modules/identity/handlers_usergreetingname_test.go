// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A body that does not name greeting_name is refused, not read as a clear: a
// null and an absent field decode alike, and only the null means "clear it".
func TestABodyWithoutTheGreetingNameIsRefusedNotCleared(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/me/greeting-name", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	Handlers{}.SaveMyGreetingName(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 — an empty body must not erase the saved name", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "greeting_name") {
		t.Errorf("the refusal does not name the field: %s", rec.Body.String())
	}
}
