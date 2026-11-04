// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListExistingCustomerNotices serves GET /existing-customer-notices.
func (h Handlers) ListExistingCustomerNotices(w http.ResponseWriter, r *http.Request) {
	notices, err := h.store.CustomerNotices(r.Context())
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	data := make([]crmcontracts.ExistingCustomerNotice, 0, len(notices))
	for _, n := range notices {
		data = append(data, wireCustomerNotice(n))
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{keyData: data})
}

// DeclareExistingCustomerNotice serves POST /existing-customer-notices.
func (h Handlers) DeclareExistingCustomerNotice(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.DeclareExistingCustomerNoticeRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	purposes := make([]ids.UUID, 0, len(req.PurposeIds))
	for _, p := range req.PurposeIds {
		purposes = append(purposes, ids.UUID(p))
	}
	notice, err := h.store.DeclareCustomerNotice(r.Context(), DeclareNoticeInput{
		LegalEntityID:     ids.UUID(req.LegalEntityId),
		CustomerGroup:     req.CustomerGroup,
		CollectionChannel: req.CollectionChannel,
		NoticeText:        req.NoticeText,
		EffectiveFrom:     req.EffectiveFrom,
		EffectiveUntil:    req.EffectiveUntil,
		PurposeIDs:        purposes,
		Now:               time.Now(),
	})
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, wireCustomerNotice(notice))
}

// WithdrawExistingCustomerNotice serves POST /existing-customer-notices/{id}/withdraw.
func (h Handlers) WithdrawExistingCustomerNotice(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.WithdrawExistingCustomerNoticeJSONBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	notice, err := h.store.WithdrawCustomerNotice(r.Context(), ids.UUID(id), req.Reason)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireCustomerNotice(notice))
}

func wireCustomerNotice(n CustomerNotice) crmcontracts.ExistingCustomerNotice {
	purposes := make([]openapi_types.UUID, 0, len(n.PurposeIDs))
	for _, p := range n.PurposeIDs {
		purposes = append(purposes, openapi_types.UUID(p))
	}
	return crmcontracts.ExistingCustomerNotice{
		Id:                openapi_types.UUID(n.ID),
		LegalEntityId:     openapi_types.UUID(n.LegalEntityID),
		CustomerGroup:     n.CustomerGroup,
		CollectionChannel: n.CollectionChannel,
		NoticeText:        n.NoticeText,
		EffectiveFrom:     n.EffectiveFrom,
		EffectiveUntil:    n.EffectiveUntil,
		PurposeIds:        purposes,
		CreatedAt:         n.CreatedAt,
		WithdrawnAt:       n.WithdrawnAt,
		WithdrawnReason:   n.WithdrawnReason,
	}
}
