// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ownershipRequestHandlers is the HTTP door onto a colleague's request to be
// handed a contact or company.
type ownershipRequestHandlers struct {
	transfers ownershipTransfers
}

// RequestOwnershipTransfer needs nothing of Idempotency-Key itself: the
// middleware claims the key and replays the first answer to a retry.
func (h ownershipRequestHandlers) RequestOwnershipTransfer(
	w http.ResponseWriter, r *http.Request, recordType string, id crmcontracts.Id, _ crmcontracts.RequestOwnershipTransferParams,
) {
	var body crmcontracts.OwnershipRequestInput
	if r.ContentLength != 0 && !httperr.Decode(w, r, &body) {
		return
	}
	note := ""
	if body.Note != nil {
		note = *body.Note
	}
	out, created, err := h.transfers.Request(r.Context(), recordType, ids.UUID(id), note)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httperr.WriteJSON(w, status, wireOwnershipRequest(out))
}

func (h ownershipRequestHandlers) GetOwnershipRequest(w http.ResponseWriter, r *http.Request, recordType string, id crmcontracts.Id) {
	out, err := h.transfers.Own(r.Context(), recordType, ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireOwnershipRequest(out))
}

func (h ownershipRequestHandlers) WithdrawOwnershipRequest(w http.ResponseWriter, r *http.Request, recordType string, id crmcontracts.Id) {
	if err := h.transfers.Withdraw(r.Context(), recordType, ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func wireOwnershipRequest(view ownershipRequestView) crmcontracts.OwnershipRequest {
	return crmcontracts.OwnershipRequest{
		ApprovalId: openapi_types.UUID(view.ApprovalID.UUID),
		RecordType: crmcontracts.OwnershipRequestRecordType(view.Transfer.RecordType),
		RecordId:   openapi_types.UUID(view.Transfer.RecordID),
		OwnerId:    openapi_types.UUID(view.Transfer.FromID),
		Status:     crmcontracts.OwnershipRequestStatus("pending"),
		ExpiresAt:  view.ExpiresAt,
	}
}
