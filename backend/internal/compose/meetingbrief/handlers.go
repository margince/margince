// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The HTTP transport for the pre-meeting brief. Wire concerns only: bind the
// path id, refuse the mode this read cannot honestly serve, and hand the result
// to the sentinel error mapping.

import (
	"net/http"

	"github.com/margince/margince/backend/internal/compose/modelfailure"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated meeting-brief stub.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetMeetingBrief implements GET /activities/{id}/meeting-brief.
func (h Handlers) GetMeetingBrief(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetMeetingBriefParams) {
	var requested *ids.ProjectID
	if params.ProjectId != nil {
		project := ids.From[ids.ProjectKind](ids.UUID(*params.ProjectId))
		requested = &project
	}
	brief, err := h.svc.GetScoped(r.Context(), ids.UUID(id), requested)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, brief)
}
