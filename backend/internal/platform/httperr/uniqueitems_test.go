// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The contract declares contact_ids uniqueItems, and the generated type carries
// that as a tag, so the shared decode refuses a contact named twice.
func TestDecodeRefusesAContactNamedTwiceInABulkSetAside(t *testing.T) {
	const id = "0b6f8f7e-1c2d-4e5f-8a9b-0c1d2e3f4a5b"
	const other = "1c7a9e8f-2d3e-4f5a-9b0c-1d2e3f4a5b6c"
	for name, tc := range map[string]struct {
		body     string
		wantCode int
	}{
		"each contact once": {`{"contact_ids":["` + id + `","` + other + `"],"days":7}`, http.StatusOK},
		"one contact twice": {`{"contact_ids":["` + id + `","` + id + `"],"days":7}`, http.StatusUnprocessableEntity},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/v1/contacts/nudge-dismissals", strings.NewReader(tc.body))
			var into crmcontracts.DismissRelationshipNudgesRequest
			if ok := Decode(rec, req, &into); ok != (tc.wantCode == http.StatusOK) {
				t.Fatalf("decoded = %v for %s", ok, tc.body)
			}
			if tc.wantCode == http.StatusOK {
				return
			}
			body := rec.Body.String()
			if rec.Code != tc.wantCode || !strings.Contains(body, `"field":"contact_ids"`) || !strings.Contains(body, `"code":"duplicate"`) {
				t.Fatalf("answered %d %s, want 422 naming contact_ids as a duplicate", rec.Code, body)
			}
		})
	}
}
