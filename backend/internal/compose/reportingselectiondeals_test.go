// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A selection narrows when it names a set smaller than the reader's whole list.
func TestWhatNarrowsAReviewSelection(t *testing.T) {
	t.Parallel()
	id := openapi_types.UUID(ids.NewV7())
	country := "DE"
	for name, c := range map[string]struct {
		selection crmcontracts.ReportingSelection
		want      bool
	}{
		"nothing":        {crmcontracts.ReportingSelection{}, false},
		"the workspace":  {crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKindWorkspace}}, false},
		"an owner":       {crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: &id}}, true},
		"a pipeline":     {crmcontracts.ReportingSelection{PipelineId: &id}, true},
		"a country":      {crmcontracts.ReportingSelection{Country: &country}, true},
		"the workspace+": {crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKindWorkspace}, Country: &country}, true},
	} {
		if got := reportingSelectionNarrows(c.selection); got != c.want {
			t.Errorf("%s: narrows = %v, want %v", name, got, c.want)
		}
	}
}

// The grant is asked before any statement: refusingTx panics if one is sent.
func TestASelectionWithNoDealGrantIsRefusedBeforeAnyQuery(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:test"})
	_, err := reportingSelectionDeals(ctx, refusingTx{}, nil, crmcontracts.ReportingSelection{})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("answered %v, want permission denied", err)
	}
}

// A scope id with no kind is refused rather than dropped, which would answer
// the whole list as though it had narrowed.
func TestAScopeIDWithoutAKindIsRefused(t *testing.T) {
	t.Parallel()
	id := openapi_types.UUID(ids.NewV7())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/analytics/decisions", nil)
	salesDecisionHandlers{}.ListSalesDecisions(w, r, crmcontracts.ListSalesDecisionsParams{ScopeId: &id})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status %d: %s", w.Code, w.Body.String())
	}
}
