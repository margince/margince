// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The transport every signed provider webhook shares: it meters the anonymous
// caller, caps the body and maps the receiver's refusals onto the wire. A network
// supplies its own body reader and the receiver that checks its signature.

import (
	"errors"
	"net/http"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

// The refusals of a webhook call, each with its own answer. The rejection names no
// reason: which check failed, or that no account is connected, would tell a
// stranger how near a forgery came.
var (
	ErrChannelEventRejected     = errors.New("capture: the webhook call is signed by no connected account")
	ErrChannelEventsUnavailable = errors.New("capture: the webhook call could not be recorded")
)

// channelEventCallsPerMinute is one sender's budget. Each call costs a secret
// decrypt per connected account and a body hash, so an anonymous caller is always
// metered.
const channelEventCallsPerMinute = 600

// webhookGate is the shared half of a network's webhook handlers. Its zero value
// answers 503, as a role that composes no receiver must.
type webhookGate struct {
	// label names the account kind in the 403 detail, such as "WhatsApp number".
	label   string
	ready   bool
	perPeer *ratelimit.Limiter
}

func newWebhookGate(name, label string, ready bool, newLimiter func(string, ratelimit.Kind, int, time.Duration) *ratelimit.Limiter) webhookGate {
	return webhookGate{
		label: label, ready: ready,
		perPeer: newLimiter("channel-events/"+name, ratelimit.FailClosed, channelEventCallsPerMinute, time.Minute),
	}
}

// allow answers whether this call may go on, and writes the refusal when not.
func (g webhookGate) allow(w http.ResponseWriter, r *http.Request) bool {
	if !g.ready {
		g.fail(w, r, ErrChannelEventsUnavailable)
		return false
	}
	if !g.perPeer.Allow(httpserver.ClientIP(r)) {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusTooManyRequests, Code: "rate_limited",
			RetryAfterSeconds: g.perPeer.RetryAfterSeconds(), Detail: "Too many calls. Send it again after the wait.",
		})
		return false
	}
	return true
}

// body reads the delivery through the network's own reader and writes the 413 or
// the rejection when it cannot. tooLarge is that reader's own sentinel.
func (g webhookGate) body(w http.ResponseWriter, r *http.Request, read func() ([]byte, error), tooLarge error) ([]byte, bool) {
	body, err := read()
	switch {
	case errors.Is(err, tooLarge):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusRequestEntityTooLarge, Code: "channel_event_too_large",
			Detail: "The body is larger than a delivery can be.",
		})
		return nil, false
	case err != nil:
		g.fail(w, r, ErrChannelEventRejected)
		return nil, false
	}
	return body, true
}

func (g webhookGate) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrChannelEventRejected):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusForbidden, Code: "channel_event_rejected",
			Detail: "The call is not from a connected " + g.label + ".",
		})
	case errors.Is(err, ErrChannelEventsUnavailable):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusServiceUnavailable, Code: "channel_events_unavailable",
			Detail: "The call could not be taken now. Send it again later.",
		})
	default:
		httperr.Write(w, r, err)
	}
}
