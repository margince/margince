// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The contract requires `kind` and `auto`, so both decode into plain values
// and an absent one is indistinguishable from its zero.
//
// Read off the struct alone, a client that forgot `auto` opted the
// installation out of applying that kind on sight, and answered 200.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAbsentAutonomyFieldIsRefusedRatherThanRead(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ body, field string }{
		"no auto":   {`{"kind":"company_name_promotion"}`, "auto"},
		"null auto": {`{"kind":"company_name_promotion","auto":null}`, "auto"},
		"no kind":   {`{"auto":true}`, "kind"},
		"neither":   {`{}`, "kind"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// A nil service is the fixture the claim needs: the guard refuses
			// before anything is reached, so reading on panics.
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/v1/autonomy", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			NewHandlers(nil).UpdateAutonomy(rec, req.WithContext(aHuman()))

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body.String())
			}
			if got := faultFields(t, rec.Body.Bytes()); !strings.Contains(got, tc.field) {
				t.Errorf("the refusal names %q, want %q", got, tc.field)
			}
		})
	}
}

// aHuman is the principal this endpoint requires, so the request reaches the
// field guards rather than stopping at the human-only gate.
func aHuman() context.Context {
	rep := ids.NewV7()
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + rep.String(), UserID: rep,
	})
}

func faultFields(t *testing.T, body []byte) string {
	t.Helper()
	var problem struct {
		Details struct {
			Errors []struct {
				Field string `json:"field"`
			} `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(body, &problem); err != nil {
		t.Fatalf("decoding the problem document: %v; body %s", err, body)
	}
	fields := make([]string, 0, len(problem.Details.Errors))
	for _, e := range problem.Details.Errors {
		fields = append(fields, e.Field)
	}
	return strings.Join(fields, ",")
}
