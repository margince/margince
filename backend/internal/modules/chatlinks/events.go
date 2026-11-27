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
	"log/slog"
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
	Authorization string
	ID            string
	Timestamp     string
	Signature     string
	Body          []byte
}

// EventReceiver verifies one delivery and acts on it. compose implements it,
// because the signature check and the queued read cross into other modules.
type EventReceiver interface {
	Receive(ctx context.Context, d Delivery) (crmcontracts.DispactChannelEventReceipt, error)
}

// EventHandlers is the push receiver's transport. A role that composes no
// receiver leaves it zero, and the route answers 503.
type EventHandlers struct {
	graph     EventReceiver
	graphPeer *ratelimit.Limiter
	teams     EventReceiver
	teamsPeer *ratelimit.Limiter
	receiver  EventReceiver
	slack     EventReceiver
	slackPeer *ratelimit.Limiter
	perPeer   *ratelimit.Limiter
}

// NewEventHandlers wires the transport over the receiver.
func NewEventHandlers(receiver EventReceiver) EventHandlers {
	return eventHandlersOn(receiver, ratelimit.New)
}

// eventHandlersOn meters through newLimiter. The meter fails closed while the
// shared count is out. Each notice costs a secret decrypt and a database read,
// so an anonymous caller is never let through unmetered.
func eventHandlersOn(receiver EventReceiver, newLimiter func(string, ratelimit.Kind, int, time.Duration) *ratelimit.Limiter) EventHandlers {
	return EventHandlers{
		receiver: receiver,
		perPeer:  newLimiter("channel-events/dispact", ratelimit.FailClosed, eventsPerMinute, time.Minute),
	}
}

// ReceiveDispactChannelEvent takes one signed notice from Dispact.
func (h EventHandlers) ReceiveDispactChannelEvent(w http.ResponseWriter, r *http.Request) {
	h.receiveEvent(w, r, h.receiver, h.perPeer, MaxEventBody, "dispact")
}

// WithSlack routes signed Slack notices through the common event boundary.
func (h EventHandlers) WithSlack(receiver EventReceiver) EventHandlers {
	h.slack = receiver
	h.slackPeer = ratelimit.New("channel-events/slack", ratelimit.FailClosed, eventsPerMinute, time.Minute)
	return h
}

// ReceiveSlackChannelEvent passes signed Slack notices to the shared event receiver.
func (h EventHandlers) ReceiveSlackChannelEvent(w http.ResponseWriter, r *http.Request) {
	h.receiveEvent(w, r, h.slack, h.slackPeer, 1<<20, "slack")
}

// WithTeams routes verified Bot Framework activities through the common event boundary.
func (h EventHandlers) WithTeams(receiver EventReceiver) EventHandlers {
	h.teams = receiver
	h.teamsPeer = ratelimit.New("channel-events/teams/bot", ratelimit.FailClosed, eventsPerMinute, time.Minute)
	return h
}

// ReceiveTeamsBotActivity validates Bot Framework activities before recording their source binding.
func (h EventHandlers) ReceiveTeamsBotActivity(w http.ResponseWriter, r *http.Request) {
	h.receiveEvent(w, r, h.teams, h.teamsPeer, 1<<20, "teams")
}

// WithTeamsGraph routes validated Graph notifications through the common event boundary.
func (h EventHandlers) WithTeamsGraph(receiver EventReceiver) EventHandlers {
	h.graph = receiver
	h.graphPeer = ratelimit.New("channel-events/teams/graph", ratelimit.FailClosed, eventsPerMinute, time.Minute)
	return h
}

// ReceiveTeamsGraphNotification validates the notification binding before reading channel changes.
func (h EventHandlers) ReceiveTeamsGraphNotification(w http.ResponseWriter, r *http.Request, params crmcontracts.ReceiveTeamsGraphNotificationParams) {
	if params.ValidationToken != nil {
		if len(*params.ValidationToken) == 0 || len(*params.ValidationToken) > 4096 {
			writeEventErr(w, r, ErrEventMalformed)
			return
		}
		if h.graph == nil || h.graphPeer == nil {
			writeEventErr(w, r, ErrEventsUnavailable)
			return
		}
		if !h.graphPeer.Allow(httpserver.ClientIP(r)) {
			httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusTooManyRequests, Code: failureRateLimited, RetryAfterSeconds: h.graphPeer.RetryAfterSeconds(), Detail: "Too many validation requests. Retry after the wait."})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if _, err := io.WriteString(w, *params.ValidationToken); err != nil {
			slog.Default().ErrorContext(r.Context(), "write notification validation response", "error", err)
		}
		return
	}
	h.receiveEvent(w, r, h.graph, h.graphPeer, 1<<20, "teams_graph")
}

func (h EventHandlers) receiveEvent(w http.ResponseWriter, r *http.Request, receiver EventReceiver, peer *ratelimit.Limiter, bodyLimit int64, provider string) {
	if receiver == nil || peer == nil {
		writeEventErr(w, r, ErrEventsUnavailable)
		return
	}
	if !peer.Allow(httpserver.ClientIP(r)) {
		ratelimit.SetHeaders(w.Header(), peer.Limit(), peer.RetryAfterSeconds())
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusTooManyRequests, Code: "rate_limited",
			RetryAfterSeconds: peer.RetryAfterSeconds(), Detail: "Too many notices. Send it again after the wait.",
		})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyLimit))
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusRequestEntityTooLarge, Code: "channel_event_too_large",
			Detail: "The body is larger than a notice can be.",
		})
		return
	}
	if err != nil {
		writeEventErr(w, r, ErrEventMalformed)
		return
	}
	delivery := Delivery{
		Authorization: r.Header.Get("Authorization"),
		ID:            r.Header.Get("webhook-id"),
		Timestamp:     r.Header.Get("webhook-timestamp"),
		Signature:     r.Header.Get("webhook-signature"),
		Body:          body,
	}
	if provider == "slack" {
		delivery.Timestamp = r.Header.Get("X-Slack-Request-Timestamp")
		delivery.Signature = r.Header.Get("X-Slack-Signature")
	}
	receipt, err := receiver.Receive(r.Context(), delivery)
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
	{ErrEventRejected, http.StatusUnauthorized, "channel_event_rejected", "The notice is not signed by the connected provider."},
	{ErrEventMalformed, http.StatusBadRequest, "channel_event_malformed", "The body is not a provider notice."},
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
