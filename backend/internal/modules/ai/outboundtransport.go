// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Timeouts bounding one outbound model call. CallCeiling is the ceiling on
// the whole call; the rest bound the legs that can stall while the ceiling is
// still far away.
//
// The ceiling alone is not enough, and the difference is not theoretical: a
// re-embed pass on this tree spent 246 seconds inside a single embedding call
// on a connection the network had silently dropped, then lost the run's River
// attempt to a peer reset on the next one. Five minutes of silence on an
// embedding is never the vendor thinking — it is a connection that will not
// answer at all, and the sooner the caller hears so the sooner it retries on a
// fresh one.
const (
	// CallCeiling bounds a single model call. Generous because premium
	// completions on long context are legitimately slow — a streamed corpus
	// extraction emits ten-thousand-token answers over minutes; per-call
	// contexts tighten it where a caller has a real deadline.
	//
	// Exported because it is not this package's private business: an HTTP
	// server that serves a synchronous model call must allow the response
	// longer than this, or it cuts the connection on an answer the AI layer
	// was still entitled to wait for. cmd/api reads it for exactly that, and a
	// test holds the two together.
	CallCeiling = 300 * time.Second

	// RouteWriteDeadline is how long a handler that CALLS a model may take to
	// write its response — set on that route alone, never on the server, whose
	// short WriteTimeout protects every other endpoint from a slow reader.
	//
	// Sized for the whole logical call rather than one request: the router may
	// spend CallCeiling on each rung of the ladder, and CompleteStructured may
	// walk that ladder more than once for a single answer (the first try, the
	// schema-invalid retry, the escalation). A deadline covering one call would
	// cut a legitimate retry — the same defect one level down from the 30s
	// server timeout that cut these responses in the first place.
	//
	// This is the same derivation railLease makes for its own lease; a round
	// number here would be a guess that happens to look like a decision.
	RouteWriteDeadline = CallCeiling*maxLadderRungs*maxLadderWalks + writeHeadroom

	// maxLadderRungs is the longest ladder any task binds — cheap_cloud then
	// premium today. Named rather than counted from a task, because this bound
	// must hold for every task that reaches these routes.
	maxLadderRungs = 2

	// writeHeadroom is the work AROUND the model calls that shares the same
	// response: assembling context, the write shape's transaction, serializing
	// the answer.
	writeHeadroom = 30 * time.Second

	// EmbedCallTimeout bounds ONE embedding call, and is the reason there is no
	// response-header timeout on the shared transport.
	//
	// A header timeout cannot tell the two kinds of call apart. A completion is
	// sent with stream:false (openai.go's Complete and every adapter beside it),
	// so the vendor holds its status line until generation FINISHES — a slow
	// reasoning model legitimately says nothing for minutes, and a header
	// deadline would cut exactly the call CallCeiling is generous for. An
	// embedding is the opposite: it is one forward pass, it answers in about a
	// second, and a minute of silence on one is never the model thinking.
	//
	// So the bound lives on the embed lane instead, where the expected duration
	// is actually known — Router.Embed applies it to every embedding call there
	// is. The ping below is what protects the completion path, by killing a dead
	// connection rather than by guessing how long an answer may take.
	EmbedCallTimeout = 60 * time.Second

	// http2PingAfterIdle asks the HTTP/2 transport to ping a connection that has
	// gone this long without a frame, and http2PingTimeout is how long the ping
	// itself may go unanswered before the connection is closed and its in-flight
	// requests fail. Without these an h2 connection dropped by a NAT or a load
	// balancer stays in the pool looking healthy, and every request handed to it
	// waits out CallCeiling. Both vendors this reaches over h2 (Cloudflare
	// fronts OpenRouter and Anthropic) drop idle connections well inside the
	// minute.
	http2PingAfterIdle = 20 * time.Second
	http2PingTimeout   = 10 * time.Second

	// idleConnTimeout retires a pooled connection that nothing has used for this
	// long, so a worker between passes reconnects rather than reaching for a
	// connection the far end has already forgotten.
	idleConnTimeout = 60 * time.Second
)

// dialTimeout bounds establishing one connection. Cloned transports inherit
// DefaultTransport's dialer settings; replacing the dialer to carry the egress
// guard means restating them, and this is the value net/http itself uses.
const dialTimeout = 30 * time.Second

// newOutboundClient is the HTTP client EVERY provider adapter calls a vendor
// with: one transport shape for all seven, so hardening the outbound path is a
// change here rather than seven changes that drift.
//
// The binding names where the call goes, and a binding is operator-supplied, so
// the client is built FROM the provider rather than shared across providers:
// its dialer carries that lane's egress guard (outboundegress.go), refusing the
// resolved address post-DNS so a name cannot smuggle one past it.
//
// One client per adapter rather than one shared package-level client, because
// the pool is per-transport and a shared pool would let one vendor's stalled
// connections crowd out another's.
func newOutboundClient(provider string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert // net/http's own DefaultTransport is a *http.Transport by construction
	transport.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: dialTimeout,
		Control:   dialGuard(egressFor(provider)),
	}).DialContext
	// NO PROXY, and this line is what decides whether the hook above means
	// anything. The clone inherits ProxyFromEnvironment, and a proxy turns the
	// check inside out: the dial goes to the PROXY's address — public, so
	// admitted — and the proxy is then asked to CONNECT to the binding's host,
	// which this side never sees. Every address dialGuard refuses is reachable
	// that way on any deployment carrying HTTPS_PROXY.
	//
	// The third writer of this rule in the tree rather than a shared helper: the
	// other two live in backend/pkg/extension (the extension surface, which a
	// module may not import) and in the hubspot overlay's own gate. What holds
	// them together is that each is a transport nobody else builds, and this one
	// is held by TestTheOutboundClientCannotBeRoutedThroughAProxy.
	transport.Proxy = nil
	transport.IdleConnTimeout = idleConnTimeout
	transport.ForceAttemptHTTP2 = true
	transport.HTTP2 = &http.HTTP2Config{
		SendPingTimeout: http2PingAfterIdle,
		PingTimeout:     http2PingTimeout,
	}
	return &http.Client{Timeout: CallCeiling, Transport: transport, CheckRedirect: refuseOffHostRedirect}
}

// refuseOffHostRedirect stops a redirect that would carry the installation's
// model key somewhere the binding did not name.
//
// Go strips the headers IT knows to be sensitive across a host change —
// Authorization, Cookie, WWW-Authenticate — and it has never heard of
// `x-api-key` or `x-goog-api-key`. Anthropic and Gemini authenticate with
// exactly those, so a vendor answering 302 to another host would be handed the
// customer's own credential by a client that believed it was being careful.
// Nor does it strip anything when only the SCHEME changes, so an https endpoint
// redirecting to http sends the same key in clear.
//
// Scoped to the hop that leaks rather than refused outright, which is where this
// differs from modellist.go's noRedirect. That one covers a list endpoint where
// a 3xx never happens in normal operation, and its comment declines to make the
// wider change here because this client also carries streaming completions,
// where a redirect a vendor genuinely uses would become an outage. A same-host
// redirect that keeps its scheme carries the key nowhere new, so it is followed;
// the two hops that move a credential are the two that are refused.
func refuseOffHostRedirect(req *http.Request, via []*http.Request) error {
	previous := via[len(via)-1]
	if !strings.EqualFold(req.URL.Hostname(), previous.URL.Hostname()) {
		return fmt.Errorf("ai: refusing a redirect from %s to %s: the model key travels with the request, and the binding named the first host",
			previous.URL.Hostname(), req.URL.Hostname())
	}
	// The PORT as well as the host, because a hostname is not an endpoint: one
	// machine serves many, and :8443 on the vendor's own host is a different
	// service from :443 — quite possibly somebody else's, on shared hosting.
	// The dial guard cannot help here either; it judges the address, and the
	// address has not changed.
	if effectivePort(previous.URL) != effectivePort(req.URL) {
		return fmt.Errorf("ai: refusing a redirect from %s to port %s on the same host: another port is another service, and it is not the one the binding named",
			previous.URL.Host, effectivePort(req.URL))
	}
	if strings.EqualFold(previous.URL.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		return fmt.Errorf("ai: refusing a redirect from https to %s on %s: the model key would leave in clear",
			req.URL.Scheme, req.URL.Hostname())
	}
	return nil
}

// effectivePort is the port a url actually dials: the one it names, or its
// scheme's default when it names none.
//
// Read rather than compared as text, because `https://vendor.example` and
// `https://vendor.example:443` are one endpoint under two spellings, and a
// redirect between them moves the request nowhere. A rule that compared
// `URL.Host` would refuse that hop and call a vendor's own normalisation an
// attack.
func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}
