// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAFailureAcknowledgementWithoutFailedAtIsRefusedAsInvalid(t *testing.T) {
	for _, body := range []string{`{}`, `{"failed_at": null}`} {
		t.Run(body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/approvals/x/failure-acknowledgement", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			Handlers{}.AcknowledgeApprovalFailure(rec, req, crmcontracts.Id{})

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), "failed_at") {
				t.Fatalf("body does not name failed_at: %s", rec.Body)
			}
		})
	}
}
