// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The WhatsApp transport. ChannelHandlers carries the admin's connect, list and
// disconnect. WhatsAppEventHandlers is Meta's anonymous webhook: it meters the
// caller, caps the body and maps the receiver's refusals onto the wire. The
// receiver in compose checks the secrets and stores the items.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The refusals of a WhatsApp webhook call, each with its own answer. The
// rejection names no reason: which check failed, or that no number is connected,
// would tell a stranger how near a forgery came.
var (
	ErrWhatsAppEventRejected     = errors.New("capture: the whatsapp call is signed by no connected number")
	ErrWhatsAppEventsUnavailable = errors.New("capture: the whatsapp call could not be recorded")
)

// whatsAppCallsPerMinute is one sender's budget. Each call costs a secret
// decrypt per connected number and a body hash, so an anonymous caller is
// always metered.
const whatsAppCallsPerMinute = 600

// WhatsAppEventReceiver checks and stores what Meta sends. compose implements
// it, because storing an item and queuing its filing cross into the job queue.
type WhatsAppEventReceiver interface {
	// Challenge returns the hub.challenge to echo when hub.verify_token is the
	// verify token of a connected number.
	Challenge(ctx context.Context, query url.Values) (string, error)
	// Receive checks signature (the X-Hub-Signature-256 header) of body against the
	// connected numbers' app secrets and stores the items that verify.
	Receive(ctx context.Context, body []byte, signature string) (crmcontracts.WhatsAppWebhookReceipt, error)
}

// WhatsAppEventHandlers is Meta's webhook transport. A role that composes no
// receiver leaves it zero, and the route answers 503.
type WhatsAppEventHandlers struct {
	receiver WhatsAppEventReceiver
	perPeer  *ratelimit.Limiter
}

// NewWhatsAppEventHandlers wires the transport over the receiver.
func NewWhatsAppEventHandlers(receiver WhatsAppEventReceiver) WhatsAppEventHandlers {
	return whatsAppEventHandlersOn(receiver, ratelimit.New)
}

func whatsAppEventHandlersOn(receiver WhatsAppEventReceiver, newLimiter func(string, ratelimit.Kind, int, time.Duration) *ratelimit.Limiter) WhatsAppEventHandlers {
	return WhatsAppEventHandlers{
		receiver: receiver,
		perPeer:  newLimiter("channel-events/whatsapp", ratelimit.FailClosed, whatsAppCallsPerMinute, time.Minute),
	}
}

// allow answers whether this call may go on, and writes the refusal when not.
func (h WhatsAppEventHandlers) allow(w http.ResponseWriter, r *http.Request) bool {
	if h.receiver == nil {
		writeWhatsAppEventErr(w, r, ErrWhatsAppEventsUnavailable)
		return false
	}
	if !h.perPeer.Allow(httpserver.ClientIP(r)) {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusTooManyRequests, Code: "rate_limited",
			RetryAfterSeconds: h.perPeer.RetryAfterSeconds(), Detail: "Too many calls. Send it again after the wait.",
		})
		return false
	}
	return true
}

// VerifyWhatsAppWebhook answers Meta's subscription check with the challenge as
// plain text.
func (h WhatsAppEventHandlers) VerifyWhatsAppWebhook(w http.ResponseWriter, r *http.Request, _ crmcontracts.VerifyWhatsAppWebhookParams) {
	if !h.allow(w, r) {
		return
	}
	challenge, err := h.receiver.Challenge(r.Context(), r.URL.Query())
	if err != nil {
		writeWhatsAppEventErr(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	//craft:ignore swallowed-errors the status line is already on the wire, so a failed write (the caller left) has no channel back
	//nolint:gosec // G705: the challenge is echoed only after a connected number's verify token matched, as nosniff text/plain
	_, _ = io.WriteString(w, challenge)
}

// ReceiveWhatsAppWebhook takes one signed delivery from Meta.
func (h WhatsAppEventHandlers) ReceiveWhatsAppWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r) {
		return
	}
	body, err := whatsapp.ReadBody(w, r)
	switch {
	case errors.Is(err, whatsapp.ErrBodyTooLarge):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusRequestEntityTooLarge, Code: "channel_event_too_large",
			Detail: "The body is larger than a delivery can be.",
		})
		return
	case err != nil:
		writeWhatsAppEventErr(w, r, ErrWhatsAppEventRejected)
		return
	}
	receipt, err := h.receiver.Receive(r.Context(), body, r.Header.Get("X-Hub-Signature-256"))
	if err != nil {
		writeWhatsAppEventErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, receipt)
}

func writeWhatsAppEventErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrWhatsAppEventRejected):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusForbidden, Code: "channel_event_rejected",
			Detail: "The call is not from a connected WhatsApp number.",
		})
	case errors.Is(err, ErrWhatsAppEventsUnavailable):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusServiceUnavailable, Code: "channel_events_unavailable",
			Detail: "The call could not be taken now. Send it again later.",
		})
	default:
		httperr.Write(w, r, err)
	}
}

// CreateWhatsAppConnection connects a WhatsApp Business number and returns its
// verify token once.
func (h ChannelHandlers) CreateWhatsAppConnection(w http.ResponseWriter, r *http.Request) {
	if !h.composed(w, r) {
		return
	}
	var req crmcontracts.CreateWhatsAppConnectionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	conn, verifyToken, err := h.store.ConnectWhatsApp(r.Context(), ConnectWhatsAppRequest{
		WABAID: req.WabaId, PhoneNumberID: req.PhoneNumberId, DisplayPhone: req.DisplayPhoneNumber,
		AccessToken: req.AccessToken, AppSecret: req.AppSecret, ReaderTeam: ids.UUID(req.ReaderTeamId),
	})
	if err != nil {
		writeChannelErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, crmcontracts.WhatsAppConnectionCreated{
		Connection: wireWhatsAppConnection(conn), VerifyToken: verifyToken,
	})
}

// ListWhatsAppConnections returns the connected WhatsApp numbers.
func (h ChannelHandlers) ListWhatsAppConnections(w http.ResponseWriter, r *http.Request) {
	if !h.composed(w, r) {
		return
	}
	conns, err := h.store.ListWhatsAppConnections(r.Context())
	if err != nil {
		writeChannelErr(w, r, err)
		return
	}
	data := make([]crmcontracts.WhatsAppConnection, 0, len(conns))
	for _, c := range conns {
		data = append(data, wireWhatsAppConnection(c))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.WhatsAppConnectionListResponse{Data: data})
}

// DisconnectWhatsAppConnection withdraws a number and destroys its secrets.
func (h ChannelHandlers) DisconnectWhatsAppConnection(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	if err := h.store.DisconnectWhatsApp(r.Context(), ids.UUID(id)); err != nil {
		writeChannelErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func wireWhatsAppConnection(c WhatsAppConnection) crmcontracts.WhatsAppConnection {
	return crmcontracts.WhatsAppConnection{
		Id:                 openapi_types.UUID(c.ID),
		WabaId:             c.WABAID,
		PhoneNumberId:      c.PhoneNumberID,
		DisplayPhoneNumber: c.DisplayPhone,
		Status:             crmcontracts.WhatsAppConnectionStatus(c.Status),
		ConnectedAt:        c.ConnectedAt,
		SeatId:             optionalUUID(c.SeatID),
		ReaderTeamId:       optionalUUID(c.ReaderTeam),
		MailVisibility:     optionalWhatsAppVisibility(c.Visibility),
	}
}

func optionalWhatsAppVisibility(v *string) *crmcontracts.WhatsAppConnectionMailVisibility {
	if v == nil {
		return nil
	}
	wire := crmcontracts.WhatsAppConnectionMailVisibility(*v)
	return &wire
}

// UpdateWhatsAppConnectionVisibility sets who may read the number's messages.
func (h ChannelHandlers) UpdateWhatsAppConnectionVisibility(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	var req crmcontracts.UpdateTelegramBusinessConnectionVisibilityRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	conn, err := h.store.ChangeWhatsAppVisibility(r.Context(), ids.UUID(id), string(req.MailVisibility), req.Confirm)
	if err != nil {
		writeChannelErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireWhatsAppConnection(conn))
}
