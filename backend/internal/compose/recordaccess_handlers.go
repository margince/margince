// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordAccessHandlers serves the two "who can see this record" reads. The
// read gates itself through the record's own read path, so this is transport.
type recordAccessHandlers struct {
	access RecordAccessReads
}

// GetContactAccess implements GET /contacts/{id}/access.
func (h recordAccessHandlers) GetContactAccess(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetContactAccessParams,
) {
	h.serve(w, r, accessTableContact, ids.UUID(id), params.Cursor, params.Limit)
}

// GetCompanyAccess implements GET /companies/{id}/access.
func (h recordAccessHandlers) GetCompanyAccess(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetCompanyAccessParams,
) {
	h.serve(w, r, accessTableCompany, ids.UUID(id), params.Cursor, params.Limit)
}

func (h recordAccessHandlers) serve(w http.ResponseWriter, r *http.Request, table string, id ids.UUID, cursor *string, limit *int) {
	out, err := h.access.Read(r.Context(), table, id, cursor, limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
