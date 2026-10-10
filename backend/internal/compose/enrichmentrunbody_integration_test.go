// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A paid lookup is bought only as the request spells it: a run that names no
// provider, or an empty list of categories, is refused before the money.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnEnrichmentRunThatLeavesOutWhatItBuysIsRefusedBeforeTheMoney(t *testing.T) {
	e := integration.Setup(t)
	contact := ids.NewV7()
	e.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Open Contact', 'manual', 'human:x')`, contact)
	spend := &countingRunService{}
	handlers := integrationsHandlers{pool: e.Pool, runs: spend}
	rep := principal.WithActor(
		principal.WithWorkspaceID(context.Background(), e.WS),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:rep", UserID: e.Rep1,
			Permissions: principal.Permissions{
				Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
				RowScope: principal.RowScopeAll,
			},
		})

	for _, tc := range []struct{ name, body, field string }{
		{"no provider", `{}`, "provider"},
		{"an empty provider", `{"provider": ""}`, "provider"},
		{"an empty list of categories", `{"provider": "surfe", "categories": []}`, "categories"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/contacts/"+contact.String()+"/enrichment-runs",
			strings.NewReader(tc.body)).WithContext(rep)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handlers.CreateContactEnrichmentRun(rec, req,
			crmcontracts.Id(openapi_types.UUID(contact)), crmcontracts.CreateContactEnrichmentRunParams{})
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.field) {
			t.Errorf("%s -> %d %s, want 422 naming %s", tc.name, rec.Code, rec.Body.String(), tc.field)
		}
	}
	if spend.calls != 0 {
		t.Errorf("QueueRun was reached %d times for a request that left out what it buys", spend.calls)
	}
}
