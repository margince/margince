// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A bad request to stop a contact, or to lift a stop, is refused above the
// transaction, so no database is needed to see what a caller is told: 422
// naming the field.
func TestAMalformedStopOrLiftIsA422NamingTheField(t *testing.T) {
	contact := openapi_types.UUID(ids.NewV7())
	stop := openapi_types.UUID(ids.NewV7())
	h := NewHandlers(nil)

	for name, c := range map[string]struct {
		body  string
		serve func(http.ResponseWriter, *http.Request)
		field string
	}{
		"a stop of a kind that is not on the list": {
			`{"kind":"bogus"}`, func(w http.ResponseWriter, r *http.Request) { h.SuppressContact(w, r, contact) }, "kind",
		},
		"a stop with a reason over the limit": {
			`{"kind":"subject_request","reason":"` + strings.Repeat("x", reasonMax+1) + `"}`,
			func(w http.ResponseWriter, r *http.Request) { h.SuppressContact(w, r, contact) }, "reason",
		},
		"lifting a stop with a blank reason": {
			`{"reason":"   "}`, func(w http.ResponseWriter, r *http.Request) { h.LiftSuppression(w, r, contact, stop) }, "reason",
		},
		"lifting a stop with a reason over the limit": {
			`{"reason":"` + strings.Repeat("x", reasonMax+1) + `"}`,
			func(w http.ResponseWriter, r *http.Request) { h.LiftSuppression(w, r, contact, stop) }, "reason",
		},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c.serve(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body)))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("answered %d (%s), want 422", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"field":"`+c.field+`"`) {
				t.Errorf("the 422 does not name %q: %s", c.field, rec.Body.String())
			}
		})
	}
}
