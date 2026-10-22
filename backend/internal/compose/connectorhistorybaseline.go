// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"net/http"
	"slices"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// noMailBaseline is what the baseline door says to a calendar connection.
const noMailBaseline = "This is a calendar connection, which brings in no mail to set a history date for."

// SetConnectorHistoryBaseline: PUT /connectors/{provider}/history-baseline.
// The registry owns the scoping and the audit, and this is wire-only.

// baselineDateField is the body key whose explicit null clears the baseline.
const baselineDateField = "date"

func (h connectorHandlers) SetConnectorHistoryBaseline(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider) {
	if h.registry == nil {
		httperr.NotImplemented(w, r, "SetConnectorHistoryBaseline")
		return
	}
	if !mailboxOnly(w, r, provider, noMailBaseline) {
		return
	}
	var req crmcontracts.SetMailHistoryBaselineRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Date == nil && !slices.Contains(httperr.ClearedFields(r), baselineDateField) {
		httperr.Write(w, r, &values.ParseError{Field: baselineDateField, Code: "date_required", Message: "send a date, or null to clear the baseline"})
		return
	}
	var date *time.Time
	timezone := ""
	if req.Date != nil {
		date = &req.Date.Time
	}
	if req.Timezone != nil {
		timezone = *req.Timezone
	}
	view, err := h.registry.SetHistoryBaseline(r.Context(), string(provider), date, timezone)
	// A mailbox this caller has not connected answers as absent, as its siblings do.
	if errors.Is(err, capture.ErrNoConnection) {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractConnection(view))
}

func historyBaselineOnWire(b *capture.HistoryBaseline) *crmcontracts.MailHistoryBaseline {
	if b == nil {
		return nil
	}
	return &crmcontracts.MailHistoryBaseline{
		Date: openapi_types.Date{Time: b.Date}, Timezone: b.Timezone, SetAt: b.SetAt,
	}
}
