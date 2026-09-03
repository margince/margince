// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ExceptionsFunc reads the open findings this caller may see.
//
// Injected rather than imported: the scope clause is over `deal`, which this
// module does not own and may not import. Applying it in the composition layer
// is also where the caller's authority already sits.
type ExceptionsFunc func(ctx context.Context, tx pgx.Tx) ([]Exception, error)

// Handlers is the assurance surface.
type Handlers struct {
	store      *Store
	exceptions ExceptionsFunc
	// now is injected so the suppression ceiling is a function of the request's
	// own clock rather than of whenever the process happened to start.
	now func() time.Time
}

// NewHandlers binds the routes to the store and its scoped read.
func NewHandlers(store *Store, exceptions ExceptionsFunc, now func() time.Time) Handlers {
	return Handlers{store: store, exceptions: exceptions, now: now}
}

// ListInputChecks answers the open findings this caller can see.
func (h Handlers) ListInputChecks(w http.ResponseWriter, r *http.Request) {
	var found []Exception
	err := h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		found, err = h.exceptions(ctx, tx)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Data []crmcontracts.InputCheck `json:"data"`
	}{Data: exceptionsToWire(found)})
}

func exceptionsToWire(in []Exception) []crmcontracts.InputCheck {
	// Empty, never nil. "Nothing to check" is a real answer and arrives shaped
	// like the array it is; null reads as "unknown", which on this surface
	// would be the difference between a clean pipeline and an unread one.
	out := make([]crmcontracts.InputCheck, 0, len(in))
	for _, e := range in {
		wire := crmcontracts.InputCheck{
			Id:            openapi_types.UUID(e.ID),
			Type:          e.Type,
			SubjectKind:   crmcontracts.InputCheckSubjectKind(e.SubjectKind),
			SubjectId:     openapi_types.UUID(e.SubjectID),
			Severity:      crmcontracts.InputCheckSeverity(e.Severity),
			Status:        crmcontracts.InputCheckStatus(e.Status),
			AffectedMinor: e.AffectedMinor,
			FirstSeenAt:   e.FirstSeenAt,
			LastSeenAt:    e.LastSeenAt,
		}
		if e.Currency != "" {
			currency := e.Currency
			wire.Currency = &currency
		}
		if e.OwnerID != nil {
			owner := openapi_types.UUID(*e.OwnerID)
			wire.OwnerId = &owner
		}
		// The structured values travel as they were stored. Decoding and
		// re-encoding them here would make this the second place that knows
		// what each exception type's keys are.
		wire.Claim = decodeSlots(e.Claim)
		wire.Observed = decodeSlots(e.Observed)
		out = append(out, wire)
	}
	return out
}

// decodeSlots renders a stored jsonb object for the wire.
//
// A value that will not decode renders as EMPTY rather than as an error: one
// malformed row must not take down the list a manager is reading before a call,
// and the row still says which deal and which check it is about.
func decodeSlots(raw []byte) *map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return &out
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return &map[string]any{}
	}
	return &out
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

// GetDataCoverage answers how current the sources behind the numbers are.
//
// Gated on `data_coverage` rather than on `forecast`: reading the pipeline's
// numbers and reading the installation's connector health are different jobs,
// and every seat that does the first does not do the second.
func (h Handlers) GetDataCoverage(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r.Context(), "data_coverage", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
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
	out := crmcontracts.DataCoverage{
		RunId: openapi_types.UUID(run.ID),
		AsOf:  run.AsOf,
		// Empty, never nil. A run that recorded no coverage is a real answer;
		// null reads as "unknown", which on this surface is the difference
		// between "nothing was tried" and "we cannot tell you".
		Sources: []crmcontracts.ForecastAssuranceSource{},
	}
	for _, c := range coverage {
		source := crmcontracts.ForecastAssuranceSource{
			Source: crmcontracts.ForecastAssuranceSourceSource(c.Source),
			State:  crmcontracts.ForecastAssuranceSourceState(c.State),
		}
		// Only a source actually read carries a date.
		if c.State == CoverageChecked {
			source.CheckedThrough = c.CheckedThrough
		}
		out.Sources = append(out.Sources, source)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
