// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// A refusal answers 400 with its parts in details, and the sentence in detail is
// still the one an agent reads: the fields are added beside it, never instead.
func TestARefusalAnswersItsPartsBesideTheSentence(t *testing.T) {
	t.Parallel()
	for _, kind := range []RefusalKind{RefusalInvalid, RefusalUnsupported, RefusalPrivacy} {
		refusal := &RefusalError{Kind: kind, Message: "what went wrong", Suggest: "what would work"}

		rec := httptest.NewRecorder()
		httperr.Write(rec, httptest.NewRequest(http.MethodPost, "/analytics/query", nil), refusal)

		var body struct {
			Code    string            `json:"code"`
			Detail  string            `json:"detail"`
			Details map[string]string `json:"details"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: the problem body is not JSON: %v", kind, err)
		}
		if rec.Code != http.StatusBadRequest || body.Code != "invalid_argument" {
			t.Errorf("%s: answered %d %q, want 400 invalid_argument", kind, rec.Code, body.Code)
		}
		if body.Detail != refusal.Error() {
			t.Errorf("%s: detail is %q, want the refusal's own sentence %q", kind, body.Detail, refusal.Error())
		}
		want := map[string]string{"kind": string(kind), "message": "what went wrong", "suggest": "what would work"}
		for key, value := range want {
			if body.Details[key] != value {
				t.Errorf("%s: details.%s is %q, want %q", kind, key, body.Details[key], value)
			}
		}
	}
}
