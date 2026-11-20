// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestOnlyAPostThatAddsMarksTheDayInsteadOfDroppingIt(t *testing.T) {
	cases := []struct {
		method, pattern string
		status          int
		adds            bool
	}{
		{"POST", "/v1/contacts", 201, true},
		{"POST", "/v1/contacts", 409, false},
		{"POST", "/v1/worklist/done", 200, false},
		{"POST", "/v1/activities/{id}/disposition", 200, false},
		{"PATCH", "/v1/activities/{id}", 200, false},
		{"DELETE", "/v1/mail-drafts/{id}", 204, false},
	}
	for _, c := range cases {
		rctx := chi.NewRouteContext()
		rctx.RoutePatterns = []string{c.pattern}
		r := httptest.NewRequest(c.method, c.pattern, nil)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		if got := onlyAdds(r, c.status); got != c.adds {
			t.Errorf("%s %s answering %d: onlyAdds = %v, want %v", c.method, c.pattern, c.status, got, c.adds)
		}
	}
}
