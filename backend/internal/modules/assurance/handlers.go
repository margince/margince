// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Handlers is the assurance surface.
type Handlers struct {
	store *Store
}

// NewHandlers binds the routes to the store.
func NewHandlers(store *Store) Handlers { return Handlers{store: store} }

// GetForecastAssurance answers the most recent completed run.
func (h Handlers) GetForecastAssurance(w http.ResponseWriter, r *http.Request) {
	run, err := h.store.LatestRun(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	coverage, err := h.store.CoverageFor(r.Context(), run.ID)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, runToWire(run, coverage))
}

func runToWire(run Run, coverage []SourceCoverage) crmcontracts.ForecastAssurance {
	out := crmcontracts.ForecastAssurance{
		RunId:         openapi_types.UUID(run.ID),
		AsOf:          run.AsOf,
		Status:        crmcontracts.ForecastAssuranceStatus(run.Status),
		EligibleDeals: run.EligibleDeals,
		// Empty, never nil. A run that recorded no coverage is a real answer
		// and arrives shaped like the array it is; null reads as "unknown",
		// which is a different claim from "nothing was tried".
		Sources: []crmcontracts.ForecastAssuranceSource{},
	}
	if run.EligibleSignals > 0 {
		signals := run.EligibleSignals
		out.EligibleSignals = &signals
	}
	if run.Readiness != nil {
		verdict := crmcontracts.ForecastAssuranceReadiness(*run.Readiness)
		out.Readiness = &verdict
	}
	for _, c := range coverage {
		source := crmcontracts.ForecastAssuranceSource{
			Source: crmcontracts.ForecastAssuranceSourceSource(c.Source),
			State:  crmcontracts.ForecastAssuranceSourceState(c.State),
		}
		// Only a source actually read carries a date. Copying one onto an
		// unread source would claim coverage that did not happen.
		if c.State == CoverageChecked {
			source.CheckedThrough = c.CheckedThrough
		}
		out.Sources = append(out.Sources, source)
	}
	return out
}
