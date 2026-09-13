// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AcceptAppliedDealChange records the human’s review against the displayed version.
func (h Handlers) AcceptAppliedDealChange(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, changeID openapi_types.UUID, _ crmcontracts.AcceptAppliedDealChangeParams) {
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	if version == nil {
		httperr.Write(w, r, httperr.Validation("If-Match", "missing_if_match", "accepting a change needs the version you last read"))
		return
	}
	if err := h.store.AcceptAppliedChange(r.Context(), ids.From[ids.DealKind](ids.UUID(id)), ids.UUID(changeID), *version); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
