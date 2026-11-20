// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestOnlyAnAddOrAWriteTheAppHidesMarksTheDayInsteadOfDroppingIt(t *testing.T) {
	cases := []struct {
		method, pattern string
		status          int
		keeps           bool
	}{
		{"POST", "/v1/contacts", 201, true},
		{"POST", "/v1/contacts", 409, false},
		{"PUT", "/v1/worklist/done", 204, true},
		{"PUT", "/v1/worklist/done", 409, false},
		{"POST", "/v1/worklist/done/undo", 204, false},
		{"PUT", "/v1/worklist/ready", 204, false},
		{"POST", "/v1/worklist/ready/undo", 204, false},
		{"PUT", "/v1/worklist/pins", 204, false},
		{"PUT", "/v1/activities/{id}/disposition", 204, true},
		{"PUT", "/v1/activities/{id}/disposition", 409, false},
		{"DELETE", "/v1/activities/{id}/disposition", 204, false},
		{"PATCH", "/v1/activities/{id}", 200, false},
		{"DELETE", "/v1/mail-drafts/{id}", 204, false},
	}
	for _, c := range cases {
		rctx := chi.NewRouteContext()
		rctx.RoutePatterns = []string{c.pattern}
		r := httptest.NewRequest(c.method, c.pattern, nil)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		if got := keepsTheDay(r, c.status); got != c.keeps {
			t.Errorf("%s %s answering %d: keepsTheDay = %v, want %v", c.method, c.pattern, c.status, got, c.keeps)
		}
	}
}
