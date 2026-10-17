// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteClaimOutcomeTellsTheThreeConflictsApart(t *testing.T) {
	cases := map[string]struct {
		outcome    claimOutcome
		code       string
		retryAfter string
	}{
		"different body":     {claimMismatch, "idempotency_key_conflict", ""},
		"first still runs":   {claimInProgress, "idempotency_key_in_progress", "1"},
		"earlier one failed": {claimFailed, "idempotency_key_failed", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/contacts", nil)
			writeClaimOutcome(rec, req, nil, nil, nil, "/v1/contacts", tc.outcome, storedResponse{})

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409", rec.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding problem body: %v", err)
			}
			if body["code"] != tc.code {
				t.Fatalf("code = %v, want %s", body["code"], tc.code)
			}
			if got := rec.Header().Get("Retry-After"); got != tc.retryAfter {
				t.Fatalf("Retry-After = %q, want %q", got, tc.retryAfter)
			}
		})
	}
}
