// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Every refusal this module's store returns reaches the wire as a 422 that
// names the field and the contract code, even when wrapped. The error carries
// its own verdict, so no handler-side list has to name it.
func TestEverySignalRefusalAnswers422NamingItsField(t *testing.T) {
	refusals := []struct {
		err         error
		field, code string
	}{
		{&RequiredFieldError{Field: "summary"}, "summary", "required"},
		{&NotResolvableError{Reason: "already resolved"}, "resolution_state", "not_resolvable"},
		{&NoWarmthError{Reason: "not resolved to a company"}, "resolution_state", "no_warmth"},
		{&InvalidSignalEntityTypeError{EntityType: "task"}, "entity_type", "invalid_entity_type"},
	}
	for _, refusal := range refusals {
		t.Run(refusal.code, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/signals", nil)
			httperr.Write(rec, req, fmt.Errorf("create signal: %w", refusal.err))

			var problem struct {
				Details struct {
					Errors []struct {
						Field string `json:"field"`
						Code  string `json:"code"`
					} `json:"errors"`
				} `json:"details"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decoding the problem body %q: %v", rec.Body.String(), err)
			}
			errs := problem.Details.Errors
			if rec.Code != http.StatusUnprocessableEntity || len(errs) != 1 ||
				errs[0].Field != refusal.field || errs[0].Code != refusal.code {
				t.Fatalf("%T → %d %s, want 422 %s on %s", refusal.err, rec.Code, rec.Body.String(), refusal.code, refusal.field)
			}
		})
	}
}
