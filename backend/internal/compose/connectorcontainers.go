// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The folders a mailbox owner may keep out of capture, on the wire.
//
// Its own file rather than a fifth handler in connectors.go: that file is about
// the connection's LIFECYCLE — connect, disconnect, posture, context tag — and
// this is a read of the mailbox behind one, which is a different subject and
// the reason the 501 and 502 arms below exist at all.

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// noMailToList is what a calendar or channel provider is told. It names the way
// forward the same way its siblings do: the folders being asked about belong to
// a mailbox, and the caller almost always has that connection too.
const noMailToList = "Folders belong to a mailbox. Ask the mail connection on this account, not the calendar one."

// ListConnectorContainers answers the folders or labels of the caller's own
// mailbox.
//
// Every refusal here is a different fact and answers as one. A provider with no
// folders is 501 — the client then renders the address and domain kinds and
// leaves the container one out, which is a smaller card rather than a broken
// one. A provider that will not answer is 502, because a stale list offered as
// current is how somebody excludes the wrong folder.
func (h connectorHandlers) ListConnectorContainers(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider) {
	if h.registry == nil {
		httperr.NotImplemented(w, r, "ListConnectorContainers")
		return
	}
	if !mailboxOnly(w, r, provider, noMailToList) {
		return
	}
	actor, ok := principal.Actor(r.Context())
	if !ok || actor.Type != principal.PrincipalHuman {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnauthorized,
			Code:   codeUnauthorized,
			Detail: "Reading your mailbox's folders is a signed-in human action.",
		})
		return
	}
	containers, err := h.registry.ListContainers(r.Context(), string(provider),
		ids.From[ids.UserKind](actor.UserID))
	switch {
	case errors.Is(err, capture.ErrContainersUnsupported):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusNotImplemented,
			Code:   "containers_unsupported",
			Detail: "This provider does not list folders.",
		})
		return
	case errors.Is(err, apperrors.ErrNotFound):
		// A mailbox this caller has not connected is not theirs to enumerate,
		// and answers as absent rather than as a refusal — the same shape the
		// posture write uses, and for the same reason: whether somebody else's
		// connection exists is not a thing to confirm.
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	case errors.Is(err, connector.ErrUnreachable):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusBadGateway,
			Code:   "provider_unreachable",
			Detail: "The provider did not answer, so the folder list could not be read.",
		})
		return
	case err != nil:
		httperr.Write(w, r, err)
		return
	}
	out := make([]crmcontracts.ConnectorContainer, 0, len(containers))
	for _, c := range containers {
		out = append(out, crmcontracts.ConnectorContainer{Id: c.ID, Name: c.Name})
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ConnectorContainers{Containers: out})
}
