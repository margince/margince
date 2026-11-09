// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// accountCoverageHandlers serves the Account coverage section.
type accountCoverageHandlers struct {
	svc accountCoverage
}

// GetAccountCoverage implements GET /analytics/account-coverage.
func (h accountCoverageHandlers) GetAccountCoverage(w http.ResponseWriter, r *http.Request, params crmcontracts.GetAccountCoverageParams) {
	out, err := h.svc.Read(r.Context(), params)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
