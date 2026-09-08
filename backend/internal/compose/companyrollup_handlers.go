// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The HTTP transport for GET /companies/{id}/hierarchy-rollup
// (RD-T04): binds the query default, calls the gated aggregate read in
// companyrollupread.go, and maps its result (and its one typed failure) onto
// the generated wire shape. No aggregation logic lives here — this file
// is pure edge + shape translation.

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// companyRollupHandlers shadows the generated GetCompanyHierarchyRollup
// stub over the gated read.
type companyRollupHandlers struct {
	pool *pgxpool.Pool
	// now is the read's clock (newServer defaults it to time.Now); the HTTP
	// surface never overrides it, but threading it as a real field — rather
	// than the read calling time.Now() internally — keeps the handler's
	// dependency honest and matches the house shape (deals.CloseDateCorrector,
	// approvals.Service).
	now func() time.Time
}

// GetCompanyHierarchyRollup implements (GET
// /companies/{id}/hierarchy-rollup). An absent scope defaults to the
// contract's "tree"; anything else reaches CompanyHierarchyRollup verbatim,
// which is the read's own refusal point for an out-of-vocabulary value.
func (h companyRollupHandlers) GetCompanyHierarchyRollup(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetCompanyHierarchyRollupParams) {
	scope := companyRollupScopeTree
	if params.Scope != nil {
		scope = string(*params.Scope)
	}

	result, err := CompanyHierarchyRollup(r.Context(), h.pool, ids.UUID(id), scope, h.now)
	if err != nil {
		// This branch survives its MessageFault twin only because it adds the
		// machine-readable details a fault cannot carry. Detail is the error's OWN
		// text, never a re-worded copy — that is what keeps both surfaces saying
		// the same thing about the same condition.
		var fxErr *FXRateUnavailableError
		if errors.As(err, &fxErr) {
			asOf := fxErr.AsOf.Format(time.DateOnly)
			httperr.Write(w, r, &httperr.DetailedError{
				Status: http.StatusUnprocessableEntity,
				Code:   "fx_rate_unavailable",
				Detail: fxErr.Error(),
				Details: map[string]any{
					"currency": fxErr.Currency, //nolint:goconst // a details-map key, coincidentally the same text as a SQL column/report-field name elsewhere in this package — not the same concept
					"as_of":    asOf,
				},
			})
			return
		}
		httperr.Write(w, r, err)
		return
	}

	httperr.WriteJSON(w, http.StatusOK, companyRollupToWire(result))
}

// companyRollupToWire renders the computed result onto the contract's
// CompanyHierarchyRollup: both money figures carry the read's
// resolved base currency, and restricted_excluded is always a real
// (possibly empty) array — CompanyRollupResult never leaves it nil.
func companyRollupToWire(result CompanyRollupResult) crmcontracts.CompanyHierarchyRollup {
	// The generated schema left restricted_excluded's item shape unnamed
	// (see api_gen.go), so its field spelling — Id, not ID — is dictated
	// by the wire contract this literal must match, not by house style.
	restricted := make([]struct {
		DisplayName string             `json:"display_name"`
		Id          openapi_types.UUID `json:"id"` //nolint:staticcheck // matches the generated CompanyHierarchyRollup.RestrictedExcluded item shape
	}, len(result.RestrictedExcluded))
	for i, n := range result.RestrictedExcluded {
		restricted[i] = struct {
			DisplayName string             `json:"display_name"`
			Id          openapi_types.UUID `json:"id"` //nolint:staticcheck // matches the generated CompanyHierarchyRollup.RestrictedExcluded item shape
		}{DisplayName: n.DisplayName, Id: openapi_types.UUID(n.ID)}
	}

	return crmcontracts.CompanyHierarchyRollup{
		RootId:                 openapi_types.UUID(result.RootID),
		Scope:                  crmcontracts.CompanyHierarchyRollupScope(result.Scope),
		WeightedPipeline:       companyRollupMoney(result.WeightedPipelineMinor, result.BaseCurrency),
		ClosedWon:              companyRollupMoney(result.ClosedWonMinor, result.BaseCurrency),
		ActivityCount30d:       result.ActivityCount30d,
		AggregatedAccountCount: result.AggregatedAccountCount,
		RestrictedExcluded:     restricted,
		ComputedAt:             result.ComputedAt,
	}
}

// companyRollupMoney renders one rollup figure as the wire Money shape — both
// fields always present, since every rollup measure is a real computed
// total in the workspace base currency, never a client-nullable input.
func companyRollupMoney(minor int64, currency string) crmcontracts.Money {
	amount := minor
	cur := currency
	return crmcontracts.Money{AmountMinor: &amount, Currency: &cur}
}
