// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The /channel-connections/telegram-business transport: an admin lists the
// Telegram Business accounts that linked the workspace's bot, approves one, or
// disconnects it. The store owns the admin gate and the write shape.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListTelegramBusinessConnections returns the Telegram Business accounts linked to the workspace's bot.
func (h ChannelHandlers) ListTelegramBusinessConnections(w http.ResponseWriter, r *http.Request) {
	if !h.composed(w, r) {
		return
	}
	links, err := h.store.ListBusinessConnections(r.Context())
	if err != nil {
		writeChannelErr(w, r, err)
		return
	}
	data := make([]crmcontracts.TelegramBusinessConnection, 0, len(links))
	for _, link := range links {
		data = append(data, wireBusinessConnection(link))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.TelegramBusinessConnectionListResponse{Data: data})
}

// ApproveTelegramBusinessConnection moves a waiting link to connected.
func (h ChannelHandlers) ApproveTelegramBusinessConnection(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	link, err := h.store.ApproveBusinessConnection(r.Context(), ids.UUID(id))
	if err != nil {
		writeChannelErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireBusinessConnection(link))
}

// DisconnectTelegramBusinessConnection withdraws the workspace's acceptance of a link.
func (h ChannelHandlers) DisconnectTelegramBusinessConnection(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	if err := h.store.DisconnectBusinessConnection(r.Context(), ids.UUID(id)); err != nil {
		writeChannelErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func wireBusinessConnection(c BusinessConnection) crmcontracts.TelegramBusinessConnection {
	return crmcontracts.TelegramBusinessConnection{
		Id:          openapi_types.UUID(c.ID),
		AccountId:   c.AccountID,
		OwnerLabel:  c.OwnerLabel,
		Status:      crmcontracts.TelegramBusinessConnectionStatus(c.Status),
		ConnectedAt: c.ConnectedAt,
		ApprovedAt:  c.ApprovedAt,
	}
}
