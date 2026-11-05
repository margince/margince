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

func claimTotal(key string) error {
	if key == "total_minor" {
		return Validation(key, "totals_derived", "money totals are computed server-side")
	}
	return nil
}

func TestDecodeClaimingAnswersAClaimedKeyInItsOwnWords(t *testing.T) {
	for name, tc := range map[string]struct {
		body     string
		wantCode int
		wantText string
	}{
		"a claimed key":        {`{"currency":"EUR","total_minor":5}`, http.StatusUnprocessableEntity, "totals_derived"},
		"a misspelt key":       {`{"currenc":"EUR"}`, http.StatusUnprocessableEntity, "unknown_field"},
		"a clean body":         {`{"currency":"EUR","intro_text":"Hello"}`, http.StatusOK, ""},
		"an empty body":        {``, http.StatusUnprocessableEntity, ""},
		"a body not an object": {`[1]`, http.StatusUnprocessableEntity, ""},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/v1/offers/x", strings.NewReader(tc.body))
			var into crmcontracts.UpdateOfferRequest
			ok := DecodeClaiming(rec, req, &into, claimTotal)
			if ok != (tc.wantCode == http.StatusOK) {
				t.Fatalf("decoded = %v for %s", ok, tc.body)
			}
			if !ok && (rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantText)) {
				t.Fatalf("answered %d %s, want %d naming %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantText)
			}
		})
	}
}
