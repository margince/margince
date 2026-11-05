// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// salesDecisionHandlers serves the decisions of a sales review.
type salesDecisionHandlers struct {
	decisions salesDecisions
}

// ListSalesDecisions implements GET /analytics/decisions.
func (h salesDecisionHandlers) ListSalesDecisions(w http.ResponseWriter, r *http.Request, params crmcontracts.ListSalesDecisionsParams) {
	var deal *ids.UUID
	if params.DealId != nil {
		id := ids.UUID(*params.DealId)
		deal = &id
	}
	var status *string
	if params.Status != nil {
		value := string(*params.Status)
		status = &value
	}
	out, err := h.decisions.List(r.Context(), deal, status)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.SalesDecisionListResponse{Data: out})
}

// RecordSalesDecision implements POST /analytics/decisions. The Idempotency-Key
// is the transport's; the open-decision match is the seam's own answer to a
// repeat.
func (h salesDecisionHandlers) RecordSalesDecision(w http.ResponseWriter, r *http.Request, _ crmcontracts.RecordSalesDecisionParams) {
	var body crmcontracts.SalesDecisionInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, created, err := h.decisions.Record(r.Context(), body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httperr.WriteJSON(w, status, out)
}

// RecordSalesDecisionOutcome implements POST /analytics/decisions/{id}/outcome.
func (h salesDecisionHandlers) RecordSalesDecisionOutcome(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.SalesDecisionOutcomeInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, err := h.decisions.Outcome(r.Context(), ids.UUID(id), body.Result)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// wireSalesReview binds what a sales review records beside its figures: the
// initiatives it narrows to and the decisions it writes as tasks.
func (s *Server) wireSalesReview(pool *pgxpool.Pool) {
	s.initiativeHandlers = initiatives.NewHandlers(newInitiativeStore(pool))
	decisions := newSalesDecisions(InstallationDB(pool))
	s.salesDecisionHandlers = salesDecisionHandlers{decisions: decisions}
	s.salesAssessmentHandlers = salesAssessmentHandlers{svc: newSalesAssessments(s.reportMetrics, decisions, InstallationDB(pool))}
	s.reportSetupHandlers = reportSetupHandlers{svc: newReportSetups(s.reportMetrics, s.dealsStore, pool)}
	s.accountCoverageHandlers = accountCoverageHandlers{svc: accountCoverage{db: InstallationDB(pool), now: time.Now}}
}
