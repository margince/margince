// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	"github.com/margince/margince/backend/internal/compose/salesprogress"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// salesAssessmentHandlers serves the Sales review's assessment.
type salesAssessmentHandlers struct {
	svc *salesAssessments
	// progress reads the Progress page the summary is written from.
	progress salesprogress.Reading
}

// AssessSalesReview implements POST /analytics/assessment.
func (h salesAssessmentHandlers) AssessSalesReview(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.SalesAssessmentRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	refresh := body.Refresh != nil && *body.Refresh
	var receipt *reporting.EvidenceExpectation
	if body.Receipt != nil {
		receipt = &reporting.EvidenceExpectation{Key: body.Receipt.Key, At: body.Receipt.At, FrameworkRevision: body.Receipt.FrameworkRevision}
	}
	out, err := h.svc.AssessView(r.Context(), body.Selection, refresh, body.View, receipt)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// SummarizeSalesProgress implements POST /analytics/progress/summary.
func (h salesAssessmentHandlers) SummarizeSalesProgress(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.SalesProgressSummaryRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	period := crmcontracts.GetSalesProgressParams{Period: crmcontracts.GetSalesProgressParamsPeriod(body.Period), Year: body.Year, Month: body.Month, ScopeId: body.ScopeId}
	if body.ScopeKind != nil {
		period.ScopeKind = new(crmcontracts.GetSalesProgressParamsScopeKind(*body.ScopeKind))
	}
	if !period.Period.Valid() {
		httperr.Write(w, r, httperr.Validation("period", "invalid", "choose month or year"))
		return
	}
	in, evaluationKey, err := h.progress.SummaryInput(r.Context(), period)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.svc.AssessProgress(r.Context(), period, evaluationKey, in, body.Refresh != nil && *body.Refresh)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// WithSalesAssessment binds the report_interpret lane that writes the Sales
// review's assessment. Unbound, the panel shows the deterministic summary.
func WithSalesAssessment(brain completer, routingVersion string) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if s.salesAssessmentHandlers.svc != nil {
			s.salesAssessmentHandlers.svc = s.salesAssessmentHandlers.svc.withLane(brain, routingVersion)
		}
	}
}
