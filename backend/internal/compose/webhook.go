// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The shared webhook chassis provider callbacks sit on: one route shape, one
// admission sequence — method guard,
// constant-time secret comparison, an optional second verifier, a bounded
// body read — and one response discipline. What differs between providers
// (secret granularity, payload durability, routing key) stays in each
// caller's WebhookSpec; the chassis enforces only the half that is
// genuinely shared, per the design's table of what cannot be.
//
// The response discipline is the reason Disposition exists: a malformed
// payload arrived and failed on our terms, so redelivering the same bytes
// forever would never help — that answers 2xx, same as success, so the
// provider stops retrying. A transient fault (a database blip, a queue
// momentarily down) is the opposite: redelivery is exactly the recovery
// mechanism, so that answers 500. A bare error cannot carry this
// difference, which is why Handle reports a Disposition alongside it.

package compose

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

// Disposition is why a webhook handler stopped, not merely whether it
// failed — the status a provider sees follows from the reason, not from a
// bare success/failure bit.
type Disposition int

const (
	// Accepted means the delivery was understood and acted on; the
	// caller's OnAccept status closes it out (Gmail answers 204).
	Accepted Disposition = iota
	// Poison means the delivery arrived but could not be understood or
	// acted on, and the fault is ours to inspect, not the provider's to
	// retry — redelivering the same bytes cannot change the outcome, so
	// this still answers 2xx.
	Poison
	// Transient means we failed to learn or act for a reason redelivery
	// can fix — a database blip, a queue momentarily down — so this
	// answers 500 and asks the provider to try again.
	Transient
)

// WebhookSpec is what a provider declares to sit on the chassis; the
// chassis owns admission and response discipline, Handle owns everything
// provider-specific (payload shape, durability, routing).
type WebhookSpec struct {
	// Provider names the webhook in logs (for example, "gmail").
	Provider string
	// MaxBody bounds the request body the chassis will read, so an
	// unauthenticated public endpoint cannot be used to exhaust memory.
	MaxBody int64
	// Secret returns the expected and presented shared secret for this
	// request (for example, an operator token in Gmail's query string) —
	// compared in constant time regardless of which shape it came from.
	Secret func(*http.Request) (want, got string)
	// Verify is an optional second admission factor layered on top of the
	// secret (Gmail's Google-signed OIDC bearer); nil skips it — the
	// chassis composes it rather than flattening it into the secret check.
	Verify func(context.Context, *http.Request) error
	// Handle acts on a body that has cleared admission and reports why it
	// stopped.
	Handle func(ctx context.Context, r *http.Request, body []byte) (Disposition, error)
	// OnAccept is the status written for Accepted — the one thing the
	// design allows to differ between providers on the success path.
	OnAccept int
	// Rate meters the edge before any work is spent on the request. nil
	// leaves it unmetered.
	Rate *WebhookRate
	// Fresh bounds how stale the request's own timestamp may be. nil leaves
	// the edge with no freshness bound, which is only safe where the payload
	// itself cannot be replayed usefully.
	Fresh *WebhookFreshness
}

// WebhookRate meters a webhook edge in two buckets, because one is not enough:
// the endpoint bucket bounds what a single sender costs this installation, and
// the client-IP bucket is what still brakes a flood spread across endpoints.
//
// Both are optional individually, so an edge can adopt one before the other —
// but an edge that meters NEITHER should leave WebhookRate nil rather than
// supply an empty one, so "unmetered" is a visible decision.
type WebhookRate struct {
	// PerIP meters on httpserver.ClientIP, which reads RemoteAddr and never a
	// caller-supplied X-Forwarded-For. That matters here: an edge keyed on a
	// header the sender writes is an edge the sender decides its own budget on.
	PerIP *ratelimit.Limiter

	// PerEndpoint meters on EndpointKey.
	PerEndpoint *ratelimit.Limiter

	// EndpointKey is the RESOLVED identity of what was addressed — never the
	// raw path. ratelimit leaves an over-long key unmetered and admitted by
	// design, so a key built from a path segment a caller chose is a self-serve
	// way off the meter. Resolve the segment against what is actually declared
	// first, and key on that.
	//
	// An empty answer means "nothing declared matches", which is metered under
	// one shared bucket rather than skipped: a flood of misses is still a flood.
	EndpointKey func(*http.Request) string
}

// WebhookFreshness bounds how far a request's own timestamp may sit from the
// receiver's clock.
//
// It is the half of replay protection this layer can hold. The other half —
// remembering which requests have already been seen — needs somewhere durable
// to remember them, which is the handler's business and not the chassis's. A
// skew bound is what makes that memory finite.
type WebhookFreshness struct {
	// At reads the request's timestamp. Parsing stays with the caller because
	// the encodings genuinely differ — HubSpot sends epoch milliseconds, the
	// extension edge sends epoch seconds — and a chassis that guessed between
	// them would read one provider's clock a thousand times wrong.
	//
	// A missing or unparseable timestamp answers false, and false is refused:
	// an edge with a freshness bound must not admit a request that declined to
	// say when it was made.
	At func(*http.Request) (time.Time, bool)

	// Skew is the permitted distance in either direction. Future-dated is
	// bounded too, or a sender with a fast clock mints requests that stay valid
	// past the window.
	Skew time.Duration

	// Now is the receiver's clock, injected so a test can hold one.
	Now func() time.Time
}

// Webhook builds the shared chassis: method guard, constant-time secret
// comparison, the optional second verifier, a bounded body read, then
// Handle — with the response discipline documented on Disposition applied
// uniformly regardless of provider. Correlate/AccessLog wrap the result at
// the mount point (routes.go), not here — this handler is the webhook
// itself.
func Webhook(spec WebhookSpec, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Before the secret compare, and deliberately: everything below this
		// costs the installation something — a SHA-256 over the secret, a
		// second-factor round trip, a body read up to MaxBody — and metering
		// after paying is metering that does not brake a flood.
		if !admitRate(spec.Rate, r) {
			log.WarnContext(r.Context(), "webhook: over the metered rate",
				"provider", spec.Provider)
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		// Digesting both values before comparing equalizes their length, so
		// ConstantTimeCompare leaks neither the secret's content nor its
		// length: a wrong guess of any size gets the identical 401, with an
		// empty body naming no connection ids for an attacker to learn from.
		want, got := spec.Secret(r)
		wantSum := sha256.Sum256([]byte(want))
		gotSum := sha256.Sum256([]byte(got))
		if subtle.ConstantTimeCompare(wantSum[:], gotSum[:]) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if spec.Verify != nil {
			if err := spec.Verify(r.Context(), r); err != nil {
				log.WarnContext(r.Context(), "webhook: second-factor verification failed",
					"provider", spec.Provider, "err", err)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, spec.MaxBody))
		if err != nil {
			// Oversized or truncated is ours to refuse, not the provider's
			// to retry: the same bytes would exceed the bound again on
			// redelivery, so this follows the poison-payload discipline.
			log.WarnContext(r.Context(), "webhook: body exceeded the bound or could not be read",
				"provider", spec.Provider, "err", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		// After the body cap and before the handler, which is where a provider
		// that already checked freshness itself had it: a stale timestamp is
		// refused before an HMAC is spent on it, and 401 rather than 400 so a
		// transient clock blip is retried by the sender rather than dropped.
		if !fresh(spec.Fresh, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		disposition, handleErr := spec.Handle(r.Context(), r, body)
		switch disposition {
		case Accepted:
			w.WriteHeader(spec.OnAccept)
		case Poison:
			if handleErr != nil {
				log.WarnContext(r.Context(), "webhook: poison payload",
					"provider", spec.Provider, "err", handleErr)
			}
			w.WriteHeader(http.StatusOK)
		case Transient:
			log.ErrorContext(r.Context(), "webhook: transient fault",
				"provider", spec.Provider, "err", handleErr)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			// A Handle that returns neither of the three declared values is
			// itself a bug in the caller, not a fact about this delivery —
			// fail the way an unrecoverable server error should.
			log.ErrorContext(r.Context(), "webhook: handler returned an unrecognized disposition",
				"provider", spec.Provider, "disposition", int(disposition))
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

// admitRate meters the request in both buckets, and spends BOTH budgets rather
// than short-circuiting on the first refusal.
//
// Short-circuiting would leave the second bucket under-counting exactly the
// traffic it exists to see: a flood from one address would be refused by the IP
// bucket and never recorded against the endpoint, so the endpoint's own budget
// would report a quiet edge throughout.
func admitRate(rate *WebhookRate, r *http.Request) bool {
	if rate == nil {
		return true
	}
	admitted := true
	if rate.PerIP != nil && !rate.PerIP.Allow(httpserver.ClientIP(r)) {
		admitted = false
	}
	if rate.PerEndpoint != nil && rate.EndpointKey != nil && !rate.PerEndpoint.Allow(rate.EndpointKey(r)) {
		admitted = false
	}
	return admitted
}

// fresh reports whether the request's timestamp is within the declared skew.
// An edge that declares no freshness is unbounded and says so by leaving the
// stage nil.
func fresh(f *WebhookFreshness, r *http.Request) bool {
	if f == nil {
		return true
	}
	at, ok := f.At(r)
	if !ok {
		return false
	}
	return withinSkew(f.Now(), at, f.Skew)
}

// withinSkew is the freshness comparison itself, shared by the chassis stage
// above and by the HubSpot receiver, which does not ride the chassis and parses
// epoch MILLISECONDS rather than seconds.
//
// Two things are easy to get wrong here and both are one-directional bugs, so
// they are held in one place rather than in each caller: the distance is
// ABSOLUTE, because a sender with a fast clock would otherwise mint requests
// that stay valid past the window; and the bound is inclusive, so a skew of
// exactly Skew is fresh rather than falling in a gap between the two callers'
// spellings.
func withinSkew(now, at time.Time, skew time.Duration) bool {
	delta := now.Sub(at)
	if delta < 0 {
		delta = -delta
	}
	return delta <= skew
}
