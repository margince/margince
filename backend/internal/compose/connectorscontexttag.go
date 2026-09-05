// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The connector-settings surface for the word a mailbox files what it captures
// under. Wire-only, like its siblings in connectors.go: the registry owns the
// scoping, the vocabulary check and the audit.

import (
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SetConnectorContextTag: PUT /connectors/{provider}/context-tag.
// The word this connector files what it captures under; the registry owns the
// scoping, the vocabulary check and the audit, and this is wire-only.
func (h connectorHandlers) SetConnectorContextTag(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider) {
	if h.registry == nil {
		httperr.NotImplemented(w, r, "SetConnectorContextTag")
		return
	}
	var req crmcontracts.SetConnectorContextTagRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	var tagID *ids.UUID
	if req.TagId != nil {
		chosen := ids.UUID(*req.TagId)
		tagID = &chosen
	}
	view, err := h.registry.SetContextTag(r.Context(), string(provider), tagID)
	// A mailbox this caller has not connected is not theirs to configure, and
	// answers as absent, for the reason the siblings above say.
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

// contextTagOnWire renders a connection's chosen word, and nothing at all for a
// connection that chose none — absence rather than an empty object, which would
// say a word was chosen and could not be read.
func contextTagOnWire(tag *capture.ContextTag) *crmcontracts.ConnectorContextTag {
	if tag == nil {
		return nil
	}
	id := openapi_types.UUID(tag.ID)
	name := tag.Name
	return &crmcontracts.ConnectorContextTag{Id: &id, Name: &name, Archived: tag.Archived}
}
