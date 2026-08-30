// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/http"
	"time"
)

// Timeouts bounding one outbound model call. requestTimeout is the ceiling on
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
	// requestTimeout bounds a single model call. Generous because premium
	// completions on long context are legitimately slow — a streamed corpus
	// extraction emits ten-thousand-token answers over minutes; per-call
	// contexts tighten it where a caller has a real deadline.
	requestTimeout = 300 * time.Second

	// EmbedCallTimeout bounds ONE embedding call, and is the reason there is no
	// response-header timeout on the shared transport.
	//
	// A header timeout cannot tell the two kinds of call apart. A completion is
	// sent with stream:false (openai.go's Complete and every adapter beside it),
	// so the vendor holds its status line until generation FINISHES — a slow
	// reasoning model legitimately says nothing for minutes, and a header
	// deadline would cut exactly the call requestTimeout is generous for. An
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
	// waits out requestTimeout. Both vendors this reaches over h2 (Cloudflare
	// fronts OpenRouter and Anthropic) drop idle connections well inside the
	// minute.
	http2PingAfterIdle = 20 * time.Second
	http2PingTimeout   = 10 * time.Second

	// idleConnTimeout retires a pooled connection that nothing has used for this
	// long, so a worker between passes reconnects rather than reaching for a
	// connection the far end has already forgotten.
	idleConnTimeout = 60 * time.Second
)

// newOutboundClient is the HTTP client EVERY provider adapter calls a vendor
// with: one transport shape for all seven, so hardening the outbound path is a
// change here rather than seven changes that drift.
//
// One client per adapter rather than one shared package-level client, because
// the pool is per-transport and a shared pool would let one vendor's stalled
// connections crowd out another's.
func newOutboundClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert // net/http's own DefaultTransport is a *http.Transport by construction
	transport.IdleConnTimeout = idleConnTimeout
	transport.ForceAttemptHTTP2 = true
	transport.HTTP2 = &http.HTTP2Config{
		SendPingTimeout: http2PingAfterIdle,
		PingTimeout:     http2PingTimeout,
	}
	return &http.Client{Timeout: requestTimeout, Transport: transport}
}
