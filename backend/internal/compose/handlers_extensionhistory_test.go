// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The handler has no database here, so a type that got past the check would
// panic on the row probe instead of answering.
func TestExtensionHistoryRefusesATypeOutsideTheContract(t *testing.T) {
	for _, entity := range []string{"bogus", "ext:finance:ledger_account", ""} {
		t.Run(entity, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/extension-history", nil)
			analyticsQueryHandlers{}.ListExtensionHistory(w, r, crmcontracts.ListExtensionHistoryParams{
				EntityType: crmcontracts.ListExtensionHistoryParamsEntityType(entity),
				EntityId:   openapi_types.UUID{1},
			})
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", w.Code)
			}
			if !strings.Contains(w.Body.String(), `"entity_type"`) {
				t.Fatalf("the answer does not name entity_type: %s", w.Body.String())
			}
		})
	}
}
