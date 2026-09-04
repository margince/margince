// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A paid lookup is refused on a contact the caller cannot open.
//
// The gate was `RequireHuman` alone, so any seat could spend money looking up
// any contact. On a capture-private row — which answers not-found on read — the
// only thing standing in the way was not knowing the id, and an id is not a
// secret.
//
// This holds the FLOOR, not the answer to what a paid lookup should require.
// Whether a spend needs write authority over the record, or a grant of its own,
// is the product question #4041 names. No reading of it permits spending on a
// record the spender cannot open.

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
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestAPaidLookupIsRefusedOnAContactTheCallerCannotOpen(t *testing.T) {
	e := integration.Setup(t)

	// Somebody else's capture-private contact: readable by its owner alone,
	// and not-found to everyone else.
	hidden := ids.NewV7()
	e.WsExec(t, `INSERT INTO person (id, full_name, owner_id, visibility, captured_by, source)
		VALUES ($1, 'Their Contact', $2, 'owner', 'connector:gmail', 'capture')`, hidden, e.Rep3)
	// And an ordinary workspace-visible one, so the refusal below is about
	// THIS record rather than about the endpoint refusing everything.
	open := ids.NewV7()
	e.WsExec(t, `INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Open Contact', 'manual', 'human:x')`, open)

	// A RECORDING stub, not NotConnected: that one's refusal renders as 404,
	// the same status an invisible contact gets, so a status comparison could
	// not tell "refused before the money" from "refused after it". What this
	// case is actually about is whether QueueRun was REACHED.
	spend := &countingRunService{}
	handlers := integrationsHandlers{pool: e.Pool, runs: spend}
	rep := principal.WithActor(
		principal.WithWorkspaceID(context.Background(), e.WS),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:rep", UserID: e.Rep1,
			Permissions: principal.Permissions{
				Objects:  map[string]principal.ObjectGrant{"person": {Read: true}},
				RowScope: principal.RowScopeAll,
			},
		})

	if status := enrichStatus(rep, t, handlers, hidden); status != http.StatusNotFound {
		t.Errorf("spending on a contact this seat cannot open answered %d, want 404 — "+
			"a different refusal would confirm the row exists to somebody who may "+
			"not read it", status)
	}
	if spend.calls != 0 {
		t.Errorf("the provider was asked %d times for a contact the caller cannot open — "+
			"the refusal has to land BEFORE the money, not after it", spend.calls)
	}

	// The floor is a floor, not a wall: a contact this seat CAN open reaches
	// the provider, which is where the spend actually lives.
	enrichStatus(rep, t, handlers, open)
	if spend.calls != 1 {
		t.Errorf("a contact this seat can open reached the provider %d times, want once — "+
			"the gate is refusing every record rather than the ones out of reach",
			spend.calls)
	}
}

// countingRunService records whether the spend was reached. It answers
// not-connected, so nothing is bought either way; the COUNT is the assertion.
type countingRunService struct{ calls int }

func (s *countingRunService) QueueRun(context.Context, provider.QueueInput) (provider.Run, error) {
	s.calls++
	return provider.Run{}, provider.ErrNotConnected
}

func (s *countingRunService) GetRun(context.Context, string, string) (provider.Run, error) {
	return provider.Run{}, provider.ErrNotConnected
}

func enrichStatus(
	ctx context.Context, t *testing.T, h integrationsHandlers, person ids.UUID,
) int {
	t.Helper()
	body := `{"provider":"apollo"}`
	req := httptest.NewRequest(http.MethodPost,
		"/v1/people/"+person.String()+"/enrichment-runs", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.CreatePersonEnrichmentRun(rec, req,
		crmcontracts.Id(openapi_types.UUID(person)), crmcontracts.CreatePersonEnrichmentRunParams{})
	return rec.Code
}
