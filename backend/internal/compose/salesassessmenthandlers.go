// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// salesAssessmentHandlers serves the Sales review's assessment.
type salesAssessmentHandlers struct {
	svc *salesAssessments
}

// AssessSalesReview implements POST /analytics/assessment.
func (h salesAssessmentHandlers) AssessSalesReview(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.SalesAssessmentRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	refresh := body.Refresh != nil && *body.Refresh
	out, err := h.svc.Get(r.Context(), body.Selection, refresh)
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
