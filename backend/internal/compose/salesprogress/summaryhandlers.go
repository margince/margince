// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"context"
	"net/http"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	"github.com/margince/margince/backend/internal/compose/salesassessment"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// SummaryHandlers serves the written summary of a Progress reading.
type SummaryHandlers struct {
	Reading Reading
	// Assess writes the summary from the figures, or serves the one it kept.
	Assess func(ctx context.Context, period crmcontracts.GetSalesProgressParams, evaluationKey string, in salesassessment.Input, refresh bool) (crmcontracts.SalesAssessment, error)
}

// SummarizeSalesProgress implements POST /analytics/progress/summary.
func (h SummaryHandlers) SummarizeSalesProgress(w http.ResponseWriter, r *http.Request) {
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
	in, evaluationKey, err := h.Reading.SummaryInput(r.Context(), period)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.Assess(r.Context(), period, evaluationKey, in, body.Refresh != nil && *body.Refresh)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
