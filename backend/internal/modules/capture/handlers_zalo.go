// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The Zalo transport. ChannelHandlers carries the admin's connect, list,
// visibility and disconnect. ZaloEventHandlers is Zalo's anonymous webhook; the
// shared gate meters the caller, caps the body and maps the receiver's refusals
// onto the wire. The receiver in compose checks the signature and stores the
// message.

import (
	"context"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ZaloEventReceiver checks and stores what Zalo sends. compose implements it,
// because storing a message and queuing its filing cross into the job queue.
type ZaloEventReceiver interface {
	// Receive checks signature (the X-ZEvent-Signature header) of body against
	// the connected accounts' secret keys and stores the message that verifies.
	Receive(ctx context.Context, body []byte, signature string) (crmcontracts.ZaloWebhookReceipt, error)
}

// ZaloEventHandlers is Zalo's webhook transport. A role that composes no
// receiver leaves it zero, and the route answers 503.
type ZaloEventHandlers struct {
	receiver ZaloEventReceiver
	gate     webhookGate
}

// NewZaloEventHandlers wires the transport over the receiver.
func NewZaloEventHandlers(receiver ZaloEventReceiver) ZaloEventHandlers {
	return zaloEventHandlersOn(receiver, ratelimit.New)
}

func zaloEventHandlersOn(receiver ZaloEventReceiver, newLimiter func(string, ratelimit.Kind, int, time.Duration) *ratelimit.Limiter) ZaloEventHandlers {
	return ZaloEventHandlers{
		receiver: receiver,
		gate:     newWebhookGate("zalo-oa", "Zalo Official Account", receiver != nil, newLimiter),
	}
}

// WithZaloEvents returns a copy that also serves Zalo's webhook.
func (h ChannelHandlers) WithZaloEvents(events ZaloEventHandlers) ChannelHandlers {
	h.ZaloEventHandlers = events
	return h
}

// ReceiveZaloOAWebhook takes one signed event from Zalo.
func (h ZaloEventHandlers) ReceiveZaloOAWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.gate.allow(w, r) {
		return
	}
	body, ok := h.gate.body(w, r, func() ([]byte, error) { return zalo.ReadBody(r) }, zalo.ErrBodyTooLarge)
	if !ok {
		return
	}
	receipt, err := h.receiver.Receive(r.Context(), body, r.Header.Get(zalo.SignatureHeader))
	if err != nil {
		h.gate.fail(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, receipt)
}

// CreateZaloOAConnection connects a Zalo Official Account.
func (h ChannelHandlers) CreateZaloOAConnection(w http.ResponseWriter, r *http.Request) {
	if !h.composed(w, r) {
		return
	}
	var req crmcontracts.CreateZaloOAConnectionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	conn, err := h.store.ConnectZaloOA(r.Context(), ConnectZaloOARequest{
		OAID: req.OaId, AppID: req.AppId, DisplayName: req.DisplayName,
		AppSecret: req.AppSecret, OASecretKey: req.OaSecretKey,
		AccessToken: req.AccessToken, RefreshToken: req.RefreshToken, ReaderTeam: ids.UUID(req.ReaderTeamId),
	})
	if err != nil {
		writeChannelErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, wireZaloOAConnection(conn))
}

// ListZaloOAConnections returns the connected Official Accounts.
func (h ChannelHandlers) ListZaloOAConnections(w http.ResponseWriter, r *http.Request) {
	if !h.composed(w, r) {
		return
	}
	conns, err := h.store.ListZaloOAConnections(r.Context())
	if err != nil {
		writeChannelErr(w, r, err)
		return
	}
	data := make([]crmcontracts.ZaloOAConnection, 0, len(conns))
	for _, c := range conns {
		data = append(data, wireZaloOAConnection(c))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ZaloOAConnectionListResponse{Data: data})
}

// UpdateZaloOAConnectionVisibility sets who may read the Official Account's messages.
func (h ChannelHandlers) UpdateZaloOAConnectionVisibility(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	var req crmcontracts.UpdateTelegramBusinessConnectionVisibilityRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	conn, err := h.store.ChangeZaloOAVisibility(r.Context(), ids.UUID(id), string(req.MailVisibility), req.Confirm)
	if err != nil {
		writeChannelErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireZaloOAConnection(conn))
}

// DisconnectZaloOAConnection withdraws an Official Account and destroys its secrets.
func (h ChannelHandlers) DisconnectZaloOAConnection(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	if err := h.store.DisconnectZaloOA(r.Context(), ids.UUID(id)); err != nil {
		writeChannelErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func wireZaloOAConnection(c ZaloOAConnection) crmcontracts.ZaloOAConnection {
	out := crmcontracts.ZaloOAConnection{
		Id:           openapi_types.UUID(c.ID),
		OaId:         c.OAID,
		AppId:        c.AppID,
		DisplayName:  c.DisplayName,
		Status:       crmcontracts.ZaloOAConnectionStatus(c.Status),
		ConnectedAt:  c.ConnectedAt,
		SeatId:       optionalUUID(c.SeatID),
		ReaderTeamId: optionalUUID(c.ReaderTeam),
	}
	if c.Visibility != nil {
		v := crmcontracts.ZaloOAConnectionMailVisibility(*c.Visibility)
		out.MailVisibility = &v
	}
	return out
}
