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

func TestASendWithoutASubjectOrBodyKeyIsRefusedBeforeItIsStaged(t *testing.T) {
	h := Handlers{}
	links := `,"links":[{"entity_type":"company","entity_id":"` + ids.NewV7().String() + `"}]`
	sends := map[string]struct {
		extra string
		send  func(http.ResponseWriter, *http.Request)
	}{
		"reply": {send: func(w http.ResponseWriter, r *http.Request) {
			h.SendEmail(w, r, crmcontracts.Id(ids.NewV7()), crmcontracts.SendEmailParams{})
		}},
		"company": {extra: links, send: func(w http.ResponseWriter, r *http.Request) {
			h.SendCompanyEmail(w, r, crmcontracts.SendCompanyEmailParams{})
		}},
	}
	missing := map[string]string{
		"subject": `{"body":"Hello","to":["buyer@example.test"]`,
		"body":    `{"subject":"Hi","to":["buyer@example.test"]`,
	}
	for name, s := range sends {
		for field, body := range missing {
			t.Run(name+" without "+field, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/v1/emails", strings.NewReader(body+s.extra+"}")).
					WithContext(humanCtx(ids.NewV7()))
				rec := httptest.NewRecorder()
				s.send(rec, req)
				if rec.Code != http.StatusUnprocessableEntity {
					t.Fatalf("answered %d, want 422: %s", rec.Code, rec.Body.String())
				}
				if !strings.Contains(rec.Body.String(), `"field":"`+field+`"`) {
					t.Fatalf("the refusal does not name %s: %s", field, rec.Body.String())
				}
			})
		}
	}
}
