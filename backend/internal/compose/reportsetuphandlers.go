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

// reportSetupHandlers serves the proposal both drawers ask for.
type reportSetupHandlers struct {
	svc *reportSetups
}

// ProposeReportSetup implements POST /analytics/setup-proposal.
func (h reportSetupHandlers) ProposeReportSetup(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.ReportSetupRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, err := h.svc.Propose(r.Context(), body)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// WithReportSetup binds the report_setup lane that reads a description into
// a proposal. Unbound, the endpoint answers 409 ai_not_configured.
func WithReportSetup(brain completer) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if s.reportSetupHandlers.svc != nil {
			s.reportSetupHandlers.svc = s.reportSetupHandlers.svc.withLane(brain)
		}
	}
}
