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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contract marks each bulk body required, so a missing one is the caller's
// omission: a 422, answered before the engine runs.
func TestABulkRouteWithNoBodyIsRefusedNotAServerFault(t *testing.T) {
	id := openapi_types.UUID(ids.NewV7())
	routes := map[string]func(http.ResponseWriter, *http.Request){
		"preview": bulkHandlers{}.PreviewBulkChange,
		"execute": func(w http.ResponseWriter, r *http.Request) {
			bulkHandlers{}.ExecuteBulkChange(w, r, crmcontracts.ExecuteBulkChangeParams{})
		},
		"undo": func(w http.ResponseWriter, r *http.Request) {
			bulkHandlers{}.UndoBulkChange(w, r, id, crmcontracts.UndoBulkChangeParams{})
		},
	}
	for name, route := range routes {
		rec := httptest.NewRecorder()
		route(rec, httptest.NewRequest(http.MethodPost, "/bulk", strings.NewReader("")))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422: %s", name, rec.Code, rec.Body)
		}
	}
}
