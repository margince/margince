// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// companyOfferedHandlers shadows the generated ListCompanyOfferedValues stub
// over the contacts store, which owns the table and the read gates.
type companyOfferedHandlers struct {
	store *contacts.Store
}

// ListCompanyOfferedValues implements GET /companies/{id}/offered-values.
func (h companyOfferedHandlers) ListCompanyOfferedValues(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	values, err := h.store.ListOfferedValues(r.Context(), ids.From[ids.CompanyKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, values)
}
