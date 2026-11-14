// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

// The /channel-events/dispact transport. Dispact posts a signed notice when a
// linked channel has new talk; the engine in compose checks the signature and
// queues the read. This layer meters the caller, caps the body and maps the
// engine's refusals onto the wire.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

// MaxEventBody bounds one notice. A notice is a few short fields, so a body
// past this is not one.
const MaxEventBody = 16 << 10

// eventsPerMinute is one sender's budget. Dispact sends one notice per busy
// channel every two minutes, so this covers several hundred busy channels.
const eventsPerMinute = 600

// The refusals of a notice, each with its own answer.
var (
	ErrEventRejected     = errors.New("chatlinks: the notice is not signed by the connected Dispact bot")
	ErrEventReplayed     = errors.New("chatlinks: this notice id was already taken")
	ErrEventMalformed    = errors.New("chatlinks: the signed body is not a notice")
	ErrEventsUnavailable = errors.New("chatlinks: the notice could not be recorded")
)

// Delivery is one signed request as it arrived.
type Delivery struct {
	ID        string
	Timestamp string
	Signature string
	Body      []byte
}

// EventReceiver verifies one delivery and acts on it. compose implements it,
// because the signature check and the queued read cross into other modules.
type EventReceiver interface {
	Receive(ctx context.Context, d Delivery) (crmcontracts.DispactChannelEventReceipt, error)
}

// EventHandlers is the push receiver's transport. A role that composes no
// receiver leaves it zero, and the route answers 503.
type EventHandlers struct {
	receiver EventReceiver
	perPeer  *ratelimit.Limiter
}

// NewEventHandlers wires the transport over the receiver.
func NewEventHandlers(receiver EventReceiver) EventHandlers {
	return eventHandlersOn(receiver, ratelimit.New)
}

// eventHandlersOn meters through newLimiter. The meter fails closed while the
// shared count is out. Each notice costs a secret decrypt and a database read,
// so an anonymous caller is never let through unmetered.
func eventHandlersOn(receiver EventReceiver, newLimiter func(string, ratelimit.Kind, int, time.Duration) *ratelimit.Limiter) EventHandlers {
	return EventHandlers{receiver: receiver,
		perPeer: newLimiter("channel-events/dispact", ratelimit.FailClosed, eventsPerMinute, time.Minute)}
}

// ReceiveDispactChannelEvent takes one signed notice from Dispact.
func (h EventHandlers) ReceiveDispactChannelEvent(w http.ResponseWriter, r *http.Request) {
	if h.receiver == nil {
		writeEventErr(w, r, ErrEventsUnavailable)
		return
	}
	if !h.perPeer.Allow(httpserver.ClientIP(r)) {
		httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusTooManyRequests, Code: "rate_limited",
			RetryAfterSeconds: h.perPeer.RetryAfterSeconds(), Detail: "Too many notices. Send it again after the wait."})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxEventBody))
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusRequestEntityTooLarge, Code: "channel_event_too_large",
			Detail: "The body is larger than a notice can be."})
		return
	}
	if err != nil {
		writeEventErr(w, r, ErrEventMalformed)
		return
	}
	receipt, err := h.receiver.Receive(r.Context(), Delivery{
		ID:        r.Header.Get("webhook-id"),
		Timestamp: r.Header.Get("webhook-timestamp"),
		Signature: r.Header.Get("webhook-signature"),
		Body:      body,
	})
	if err != nil {
		writeEventErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, receipt)
}

// eventRefusals map each refusal to its answer. The rejection names no
// reason. Which check failed, or that no secret is set, would tell a stranger
// how near a forgery came.
var eventRefusals = []struct {
	err    error
	status int
	code   string
	detail string
}{
	{ErrEventRejected, http.StatusUnauthorized, "channel_event_rejected", "The notice is not signed by the connected Dispact bot."},
	{ErrEventReplayed, http.StatusConflict, "channel_event_replayed", "This notice was already taken."},
	{ErrEventMalformed, http.StatusBadRequest, "channel_event_malformed", "The body is not a Dispact notice."},
	{ErrEventsUnavailable, http.StatusServiceUnavailable, "channel_events_unavailable", "The notice could not be taken now. Send it again later."},
}

func writeEventErr(w http.ResponseWriter, r *http.Request, err error) {
	for _, refusal := range eventRefusals {
		if errors.Is(err, refusal.err) {
			httperr.Write(w, r, &httperr.DetailedError{Status: refusal.status, Code: refusal.code, Detail: refusal.detail})
			return
		}
	}
	httperr.Write(w, r, err)
}
