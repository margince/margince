// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package editaccess

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the HTTP door onto a colleague's own request for edit access.
type Handlers struct {
	Requests Engine
}

// RequestEditAccess needs nothing of Idempotency-Key itself: the middleware
// claims the key and replays the first answer to a retry.
func (h Handlers) RequestEditAccess(
	w http.ResponseWriter, r *http.Request, recordType string, id crmcontracts.Id, _ crmcontracts.RequestEditAccessParams,
) {
	out, created, err := h.Requests.Ask(r.Context(), recordType, ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httperr.WriteJSON(w, status, wire(out))
}

// GetEditAccessRequest answers the caller's open request on the record.
func (h Handlers) GetEditAccessRequest(w http.ResponseWriter, r *http.Request, recordType string, id crmcontracts.Id, _ crmcontracts.GetEditAccessRequestParams) {
	out, err := h.Requests.Own(r.Context(), recordType, ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wire(out))
}

// WithdrawEditAccessRequest takes the caller's open request back.
func (h Handlers) WithdrawEditAccessRequest(w http.ResponseWriter, r *http.Request, recordType string, id crmcontracts.Id) {
	if err := h.Requests.Withdraw(r.Context(), recordType, ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func wire(view Request) crmcontracts.EditAccessRequest {
	return crmcontracts.EditAccessRequest{
		ApprovalId: openapi_types.UUID(view.ApprovalID.UUID),
		RecordType: crmcontracts.EditAccessRequestRecordType(view.Ask.RecordType),
		RecordId:   openapi_types.UUID(view.Ask.RecordID),
		OwnerId:    openapi_types.UUID(view.Ask.OwnerID),
		Status:     crmcontracts.EditAccessRequestStatus("pending"),
		ExpiresAt:  view.ExpiresAt,
	}
}
