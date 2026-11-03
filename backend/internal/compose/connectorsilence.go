// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func (h connectorHandlers) DismissConnectorSilence(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider) {
	if h.registry == nil {
		httperr.NotImplemented(w, r, "DismissConnectorSilence")
		return
	}
	var req crmcontracts.DismissCaptureSilenceRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	view, err := h.registry.DismissSilence(r.Context(), string(provider), req.Since)
	if errors.Is(err, capture.ErrNoConnection) {
		err = apperrors.ErrNotFound
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractConnection(view))
}
