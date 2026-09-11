// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The door a designated human sends a refused message through.
//
// Thin transport. Every rule lives behind it: consent owns who may decide and
// what a decision must say, activities owns the send, and directedsend.go holds
// the order between them. This decodes, calls, and writes what came back.
//
// AT THIS LAYER because the operation spans two modules that may not import
// each other. A consent handler could record the decision and would have no way
// to send; an activities handler could send and would have no way to know
// anybody had decided.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// directedSendHandlers is the transport for a recorded decision being carried
// out.
type directedSendHandlers struct {
	// directed is nil in a composition with no send path wired, and then this
	// route answers not-implemented like every other unwired surface rather
	// than panicking on a message nobody can send.
	directed *directedSendService
}

// DirectCommunicationSend records the decision and sends the message it was
// about.
func (h directedSendHandlers) DirectCommunicationSend(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id,
) {
	if h.directed == nil {
		httperr.NotImplemented(w, r, "DirectCommunicationSend")
		return
	}
	var body crmcontracts.DirectCommunicationSendRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	sent, err := h.directed.DirectAndSend(r.Context(), ids.UUID(id), consent.DirectInput{
		ReasonCode:     string(body.ReasonCode),
		Explanation:    body.Explanation,
		WarningVersion: body.WarningVersion,
		Acknowledged:   body.Acknowledged,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, sent)
}
