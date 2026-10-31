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
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ExceptionsFunc reads the open findings this caller may see.
//
// Injected rather than imported: the scope clause is over `deal`, which this
// module does not own and may not import. Applying it in the composition layer
// is also where the caller's authority already sits.
type ExceptionsFunc func(ctx context.Context, tx pgx.Tx, population Population) ([]Exception, error)

// Population narrows a list to the deals of one team, owner or the whole
// workspace. The zero value narrows nothing beyond what the caller may see.
type Population struct {
	Kind string
	ID   *ids.UUID
}

// Handlers is the assurance surface.
type Handlers struct {
	store      *Store
	exceptions ExceptionsFunc
	// now is injected so the suppression ceiling is a function of the request's
	// own clock rather than of whenever the process happened to start.
	now      func() time.Time
	settings *config.Config
	locks    func(surface []config.Any) *[]crmcontracts.SettingLock
	preview  PreviewFunc
}

// NewHandlers binds the routes to the store and its scoped read.
func NewHandlers(store *Store, exceptions ExceptionsFunc, now func() time.Time) Handlers {
	return Handlers{store: store, exceptions: exceptions, now: now}
}

// ListInputChecks answers the open findings this caller can see.
func (h Handlers) ListInputChecks(w http.ResponseWriter, r *http.Request, params crmcontracts.ListInputChecksParams) {
	population := Population{ID: (*ids.UUID)(params.ScopeId)}
	if params.ScopeKind != nil {
		population.Kind = string(*params.ScopeKind)
	}
	var found []Exception
	var answerable map[ids.UUID]bool
	err := h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if found, err = h.exceptions(ctx, tx, population); err != nil {
			return err
		}
		answerable, err = AnswerableFindings(ctx, tx, found)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, InputCheckListToWire(found, answerable))
}

// InputCheckList is the listInputChecks answer: the findings, each with where
// it is reviewed, and their groups and totals over the whole list.
type InputCheckList struct {
	Data   []crmcontracts.InputCheck      `json:"data"`
	Groups []crmcontracts.InputCheckGroup `json:"groups"`
	Totals crmcontracts.InputCheckTotals  `json:"totals"`
}

// InputCheckListToWire renders a list and what the caller may answer in it.
// The route and its model tool both answer with it.
func InputCheckListToWire(found []Exception, answerable map[ids.UUID]bool) InputCheckList {
	data := ExceptionsToWire(found)
	for i, e := range found {
		destination := reviewDestination(answerable[e.ID])
		data[i].Destination = &destination
	}
	groups, totals := GroupFindings(found, answerable)
	out := InputCheckList{
		Data:   data,
		Groups: make([]crmcontracts.InputCheckGroup, 0, len(groups)),
		Totals: crmcontracts.InputCheckTotals{Findings: totals.Findings, Deals: totals.Deals, OtherFindings: totals.OtherFindings},
	}
	for _, g := range groups {
		exposure := make([]crmcontracts.InputCheckExposure, 0, len(g.Exposure))
		for _, money := range g.Exposure {
			exposure = append(exposure, crmcontracts.InputCheckExposure{Currency: money.Currency, AmountMinor: money.AmountMinor})
		}
		out.Groups = append(out.Groups, crmcontracts.InputCheckGroup{
			Type: g.Type, Findings: g.Findings, Deals: g.Deals, OtherFindings: g.OtherFindings,
			Exposure: exposure, Destination: reviewDestination(g.Answerable),
		})
	}
	return out
}

func reviewDestination(answerable bool) crmcontracts.InputCheckDestination {
	mode := crmcontracts.InputCheckDestinationModeRead
	if answerable {
		mode = crmcontracts.InputCheckDestinationModeAnswer
	}
	return crmcontracts.InputCheckDestination{Screen: crmcontracts.InputCheckDestinationScreenForecastReview, Mode: mode}
}

// Signal findings open their verified company; the signal remains the review subject.
func subjectOf(e Exception) *crmcontracts.AttentionSubject {
	if e.SubjectKind == subjectKindSignal {
		slots := decodeSlots(e.Claim)
		company, ok := (*slots)["company_id"].(string)
		if !ok || e.SubjectLabel == "" {
			return nil
		}
		id, err := ids.Parse(company)
		if err != nil {
			return nil
		}
		return &crmcontracts.AttentionSubject{Type: crmcontracts.AttentionSubjectTypeCompany, Id: openapi_types.UUID(id), Label: &e.SubjectLabel}
	}
	kind, ok := subjectKinds[e.SubjectKind]
	if !ok || e.SubjectLabel == "" {
		return nil
	}
	label := e.SubjectLabel
	return &crmcontracts.AttentionSubject{
		Type:  kind,
		Id:    openapi_types.UUID(e.SubjectID),
		Label: &label,
	}
}

var subjectKinds = map[string]crmcontracts.AttentionSubjectType{
	"deal": crmcontracts.AttentionSubjectTypeDeal,
}

// ExceptionsToWire renders findings on the listInputChecks item shape; the
// team weekly carries the same items.
func ExceptionsToWire(in []Exception) []crmcontracts.InputCheck {
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
			InputsChanged: e.InputsChanged,
		}
		if e.Currency != "" {
			currency := e.Currency
			wire.Currency = &currency
		}
		if e.OwnerID != nil {
			owner := openapi_types.UUID(*e.OwnerID)
			wire.OwnerId = &owner
		}
		wire.Subject = subjectOf(e)
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
		Sources:   CoverageToWire(coverage),
		Freshness: FreshnessToWire(run.Freshness),
	}
	if run.EligibleSignals > 0 {
		signals := run.EligibleSignals
		out.EligibleSignals = &signals
	}
	if run.Readiness != nil {
		verdict := crmcontracts.ForecastAssuranceReadiness(*run.Readiness)
		out.Readiness = &verdict
	}
	return out
}

// FreshnessToWire renders what changed after a reading. A reading no run has
// recorded the inputs of is unknown, never current.
func FreshnessToWire(f Freshness) crmcontracts.AssuranceFreshness {
	state := crmcontracts.AssuranceFreshnessStateCurrent
	switch {
	case !f.Known:
		state = crmcontracts.AssuranceFreshnessStateUnknown
	case f.OutOfDate():
		state = crmcontracts.AssuranceFreshnessStateOutOfDate
	}
	return crmcontracts.AssuranceFreshness{
		State: state, ChangedDeals: f.ChangedDeals, ChangedSignals: f.ChangedSignals,
		WorkspaceChanged: f.WorkspaceChanged, ChangedSince: f.FirstChangedAt,
	}
}

// CoverageToWire renders one run's source health.
//
// ONE spelling, for the three surfaces that publish it — the assurance panel,
// the operator's coverage view, and the preview a human reads before starting
// the cycle. The rule it carries is the reason it is not inlined at each: only
// a source actually READ carries a date, and a date copied onto an unread one
// claims coverage that did not happen. Three copies of that rule is three
// chances for one of them to stop being true.
//
// Empty, never nil. A run that recorded no coverage is a real answer and
// arrives shaped like the array it is; null reads as "unknown", which is a
// different claim from "nothing was tried".
func CoverageToWire(coverage []SourceCoverage) []crmcontracts.ForecastAssuranceSource {
	out := make([]crmcontracts.ForecastAssuranceSource, 0, len(coverage))
	for _, c := range coverage {
		source := crmcontracts.ForecastAssuranceSource{
			Source: crmcontracts.ForecastAssuranceSourceSource(c.Source),
			State:  crmcontracts.ForecastAssuranceSourceState(c.State),
		}
		if c.State == CoverageChecked {
			source.CheckedThrough = c.CheckedThrough
		}
		out = append(out, source)
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
		DealID:      (*ids.UUID)(body.DealId),
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
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.DataCoverage{
		RunId:   openapi_types.UUID(run.ID),
		AsOf:    run.AsOf,
		Sources: CoverageToWire(coverage),
	})
}
