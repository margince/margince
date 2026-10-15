// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"net"
	"net/http"
	"time"

	"github.com/margince/margince/backend/internal/platform/netguard"
)

// HTTPDoer is the delivery transport seam. Production wires a netguard-
// guarded client (NewGuardedClient); tests inject a loopback-permitting
// client, because netguard by design refuses the 127.0.0.1 an httptest
// receiver listens on. This is the documented testability seam — the
// guard itself is pinned by a dedicated SSRF test on NewGuardedClient.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

const (
	// deliveryTimeout bounds one attempt end-to-end; a slow or hung
	// receiver must not pin a worker goroutine.
	deliveryTimeout = 10 * time.Second
	// maxResponseBytes caps how much of a receiver's response body is
	// read: the body is only inspected for diagnostics, so a hostile
	// endpoint cannot exhaust memory by streaming forever.
	maxResponseBytes = 8 << 10
)

// NewGuardedClient builds the production delivery client. A tenant's target
// URL is dialed only if it resolves to a public address. The check runs after
// DNS on the concrete IP, so a rebind cannot bypass it (netguard.RefusePrivate).
func NewGuardedClient() *http.Client {
	dialer := &net.Dialer{Timeout: deliveryTimeout, Control: netguard.RefusePrivate}
	return &http.Client{
		Timeout: deliveryTimeout,
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: deliveryTimeout,
		},
		// A redirect is never followed. Go would turn the POST into a GET with
		// no body, and carry custom headers to a host nobody named. The 3xx is
		// recorded as the failed attempt it is.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
