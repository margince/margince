// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AnswerCalendarInvitation accepts, tentatively accepts or declines the event
// an emailed invitation names, on the caller's own calendar.
func (h Handlers) AnswerCalendarInvitation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.AnswerCalendarInvitationRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	answer, err := h.store.AnswerCalendarInvitation(r.Context(), pathID[ids.ActivityKind](id), req.Response)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, answer)
}
