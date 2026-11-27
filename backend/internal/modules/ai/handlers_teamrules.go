// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// TeamRulesHandlers serves the team rules over HTTP. Store is nil in a
// deployment without the admin store, which answers 501.
type TeamRulesHandlers struct{ Store *AdminStore }

// GetAiTeamRules answers the saved team rules to a seat holding the read grant.
func (h TeamRulesHandlers) GetAiTeamRules(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httperr.NotImplemented(w, r, "GetAiTeamRules")
		return
	}
	out, err := h.Store.ReadTeamRules(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ReplaceAiTeamRules saves new team rules for a signed-in admin or ops seat.
func (h TeamRulesHandlers) ReplaceAiTeamRules(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httperr.NotImplemented(w, r, "ReplaceAiTeamRules")
		return
	}
	var change crmcontracts.AiTeamRulesChange
	if !httperr.Decode(w, r, &change) {
		return
	}
	out, err := h.Store.ReplaceTeamRules(r.Context(), change)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
