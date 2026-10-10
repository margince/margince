// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contract requires `subject` on both send requests, so a body that leaves
// the key out is refused before any send or schedule is staged.
func TestASendWithoutASubjectKeyIsRefusedBeforeItIsStaged(t *testing.T) {
	h := Handlers{}
	company := ids.NewV7().String()
	cases := map[string]struct {
		body string
		send func(http.ResponseWriter, *http.Request)
	}{
		"reply": {
			body: `{"body":"Hello","to":["buyer@example.test"]}`,
			send: func(w http.ResponseWriter, r *http.Request) {
				h.SendEmail(w, r, crmcontracts.Id(ids.NewV7()), crmcontracts.SendEmailParams{})
			},
		},
		"company": {
			body: `{"body":"Hello","to":["buyer@example.test"],"links":[{"entity_type":"company","entity_id":"` + company + `"}]}`,
			send: func(w http.ResponseWriter, r *http.Request) {
				h.SendCompanyEmail(w, r, crmcontracts.SendCompanyEmailParams{})
			},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/emails", strings.NewReader(c.body)).
				WithContext(humanCtx(ids.NewV7()))
			rec := httptest.NewRecorder()
			c.send(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("answered %d, want 422: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"field":"subject"`) {
				t.Fatalf("the refusal does not name the subject field: %s", rec.Body.String())
			}
		})
	}
}
