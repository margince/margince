// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Handlers serves /personal-channels for the caller's own connection.
type Handlers struct {
	store *Store
}

// NewHandlers serves store's connection, replies and uploads.
func NewHandlers(store *Store) Handlers {
	return Handlers{store: store}
}

func connectionWire(c Connection) crmcontracts.PersonalChannelConnection {
	return crmcontracts.PersonalChannelConnection{
		Enabled:              c.Enabled,
		RepliesWaiting:       c.RepliesWaiting,
		OldestReplyWaitingAt: c.OldestReplyWaitingAt,
		LastPullAt:           c.LastPullAt,
	}
}

// GetPersonalChannelConnection implements (GET /personal-channels/connection).
func (h Handlers) GetPersonalChannelConnection(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.Connection(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, connectionWire(c))
}

// UpdatePersonalChannelConnection implements (PATCH /personal-channels/connection).
func (h Handlers) UpdatePersonalChannelConnection(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.UpdatePersonalChannelConnectionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	c, err := h.store.SetEnabled(r.Context(), req.Enabled)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, connectionWire(c))
}
