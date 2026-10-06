// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Both refusals are answered before the store is reached, so a Handlers with
// no store proves nothing was written.
func TestAContractWithAnUnknownFieldIsRefusedNamingIt(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/weekly-plans/current/contract",
		strings.NewReader(`{"risk":"typo"}`))

	Handlers{}.SetWeeklyPlanContract(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, `unknown_field`) || !strings.Contains(body, "risk") {
		t.Errorf("the refusal does not name the unknown field: %s", body)
	}
}

func TestAHelpRequestWithoutTheFieldIsNotAWithdrawal(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/weekly-plans/commitments/x/help", strings.NewReader(`{}`))

	Handlers{}.AskForWeeklyPlanHelp(rec, req, openapi_types.UUID(ids.NewV7()))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"required"`) {
		t.Errorf("the refusal does not say help_requested is required: %s", rec.Body)
	}
}
