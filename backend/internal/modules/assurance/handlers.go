// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the assurance surface.
type Handlers struct {
	store *Store
	// now is injected so the suppression ceiling is a function of the request's
	// own clock rather than of whenever the process happened to start.
	now func() time.Time
}

// NewHandlers binds the routes to the store.
func NewHandlers(store *Store, now func() time.Time) Handlers {
	return Handlers{store: store, now: now}
}

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

// ResolveInputCheck records somebody's answer to a finding.
func (h Handlers) ResolveInputCheck(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID,
) {
	var body crmcontracts.ResolveInputCheck
	if !httperr.Decode(w, r, &body) {
		return
	}
	in := Resolution{
		Outcome:     string(body.Outcome),
		Reason:      derefString(body.Reason),
		EvidenceRef: derefString(body.EvidenceRef),
		RemindAt:    body.RemindAt,
		ExpiresAt:   body.ExpiresAt,
	}
	if err := h.store.Resolve(r.Context(), ids.UUID(id), in, h.now()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
