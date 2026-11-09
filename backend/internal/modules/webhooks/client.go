// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"crypto/tls"
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
	return NewDeliveryClient(dialer.DialContext, nil)
}

// DialFunc opens the TCP connection a delivery is sent over.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// NewDeliveryClient builds a delivery client over dial. Production calls it
// through NewGuardedClient; a test dials its loopback receiver instead.
//
// The row is re-checked on each new connection, before any request byte
// (checkedDialer). Keep-alives are off, so no attempt rides another's check.
// HTTP/2 is off, because its flow control could hold a checked body back.
func NewDeliveryClient(dial DialFunc, tlsConfig *tls.Config) *http.Client {
	d := checkedDialer{dial: dial, tlsConfig: tlsConfig}
	return &http.Client{
		Timeout: deliveryTimeout,
		Transport: &http.Transport{
			DialContext:       d.dialPlain,
			DialTLSContext:    d.dialTLS,
			DisableKeepAlives: true,
			TLSNextProto:      map[string]func(string, *tls.Conn) http.RoundTripper{},
		},
		// A redirect is never followed. Go would turn the POST into a GET with
		// no body, and carry custom headers to a host nobody named. The 3xx is
		// recorded as the failed attempt it is.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
