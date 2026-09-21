// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The litigation-hold transport: the controller's review surface over what a
// hold is preserving, and the two decisions that place and lift one.

import (
	"context"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LegalHoldWriter places or lifts a hold on one record.
//
// A port for the reason ChangeRestorer is one: the flag sits on five tables
// owned by three modules, a module may not write a sibling's table, and compose
// owns the seam that binds them. Writing them here would make privacy a sixth
// writer of records it does not own.
type LegalHoldWriter interface {
	SetLegalHold(ctx context.Context, entityType string, id ids.UUID, held bool, reason string) error
}

// WithLegalHoldWriter wires the hold seam. Without one the routes refuse
// rather than half-serving: an install that has not been given the seam cannot
// place a hold, and saying so is honest where a 500 would not be.
func (h Handlers) WithLegalHoldWriter(holds LegalHoldWriter) Handlers {
	h.holds = holds
	return h
}

// ListLegalHolds implements (GET /retention/legal-holds).
func (h Handlers) ListLegalHolds(w http.ResponseWriter, r *http.Request, params crmcontracts.ListLegalHoldsParams) {
	page, err := ListLegalHolds(r.Context(), h.db, params.Cursor, params.Limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	data := make([]crmcontracts.HeldRecord, 0, len(page.Records))
	for _, record := range page.Records {
		data = append(data, crmcontracts.HeldRecord{
			EntityType: crmcontracts.HeldEntityType(record.EntityType),
			RecordId:   openapi_types.UUID(record.RecordID),
			Label:      record.Label,
		})
	}
	resp := struct {
		Data []crmcontracts.HeldRecord `json:"data"`
		Page crmcontracts.PageInfo     `json:"page"`
	}{Data: data, Page: crmcontracts.PageInfo{HasMore: page.HasMore}}
	if page.NextCursor != "" {
		resp.Page.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, resp)
}

// PlaceLegalHold implements
// (POST /retention/legal-holds/{entityType}/{recordId}/place).
func (h Handlers) PlaceLegalHold(w http.ResponseWriter, r *http.Request,
	entityType crmcontracts.HeldEntityType, recordID openapi_types.UUID,
	_ crmcontracts.PlaceLegalHoldParams,
) {
	h.setHold(w, r, entityType, recordID, true)
}

// LiftLegalHold implements
// (POST /retention/legal-holds/{entityType}/{recordId}/lift).
func (h Handlers) LiftLegalHold(w http.ResponseWriter, r *http.Request,
	entityType crmcontracts.HeldEntityType, recordID openapi_types.UUID,
	_ crmcontracts.LiftLegalHoldParams,
) {
	h.setHold(w, r, entityType, recordID, false)
}

// setHold is both decisions. They differ only in the direction, and a second
// spelling of the refusal and the reason check is how the two would come to
// disagree about what a stated reason is.
func (h Handlers) setHold(w http.ResponseWriter, r *http.Request,
	entityType crmcontracts.HeldEntityType, recordID openapi_types.UUID, held bool,
) {
	reason, ok := decodeStatedReason(w, r)
	if !ok {
		return
	}
	if h.holds == nil {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	if err := h.holds.SetLegalHold(
		r.Context(), string(entityType), ids.UUID(recordID), held, reason.String(),
	); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
