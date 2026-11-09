// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (h Handlers) ListContactOverrides(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListContactOverridesParams) {
	limit, cursor := 0, ""
	if params.Limit != nil {
		limit = int(*params.Limit)
	}
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	rows, page, err := h.store.ListContactOverrides(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)), limit, cursor)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	info := crmcontracts.PageInfo{HasMore: page.HasMore}
	if page.NextCursor != "" {
		info.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.StandingOverridesPage{Data: rows, Page: info})
}
