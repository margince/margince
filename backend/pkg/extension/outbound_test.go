// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The guard runs where it has to: on the concrete address, at the connect.
func TestRefuseNonPublicAnswersOnTheAddressItIsAboutToDial(t *testing.T) {
	for address, public := range map[string]bool{
		// Routable, and the only class that may be dialed.
		"93.184.216.34":     true,
		"2606:2800:220:1::": true,
		// Loopback, private, link-local — the obvious three.
		"127.0.0.1":       false,
		"10.0.0.5":        false,
		"192.168.1.10":    false,
		"172.16.4.4":      false,
		"169.254.169.254": false,
		"::1":             false,
		"fe80::1":         false,
		// The ranges the stdlib predicates miss, and the reason the list of
		// CIDRs exists at all: each of these reads as ordinary public space to
		// IsPrivate.
		"0.0.0.0":              false,
		"0.1.2.3":              false,
		"100.64.0.1":           false, // CGNAT
		"192.0.0.1":            false, // protocol assignment
		"192.0.2.5":            false, // documentation
		"198.18.0.1":           false, // benchmarking
		"198.51.100.5":         false,
		"203.0.113.5":          false,
		"240.0.0.1":            false, // reserved
		"255.255.255.255":      false,
		"192.88.99.1":          false, // 6to4 relay anycast
		"2001:db8::1":          false, // documentation
		"3fff::1":              false, // documentation, the newer range
		"64:ff9b::a9fe:a9fe":   false, // NAT64 onto the metadata address
		"64:ff9b:1::a9fe:a9fe": false, // local-use NAT64 onto the same address
		"2002:7f00:1::1":       false, // 6to4 carrying 127.0.0.1
		"2001::1":              false, // Teredo, inside the 2001::/23 blanket
		"100::1":               false, // discard-only
		"100:0:0:1::1":         false, // dummy prefix
		"5f00::1":              false, // SRv6 SIDs
		"fec0::1":              false, // deprecated site-local
		"::ffff:0:a9fe:a9fe":   false, // IPv4-translated onto the metadata address
		"::ffff:127.0.0.1":     false, // IPv4-mapped loopback
	} {
		address := address
		t.Run(address, func(t *testing.T) {
			if net.ParseIP(address) == nil {
				t.Fatalf("%q is not an address — the fixture is wrong, which would make this row pass for free", address)
			}
			err := RefuseNonPublic("tcp", net.JoinHostPort(address, "443"), nil)
			if public && err != nil {
				t.Errorf("a routable address was refused: %v", err)
			}
			if !public && err == nil {
				t.Error("dialled, and it must not")
			}
		})
	}
}

// A hook wired where it does not belong sees a name rather than a literal.
// Admitting that would be admitting an address nothing checked, so it refuses.
func TestRefuseNonPublicRefusesAnAddressItCannotRead(t *testing.T) {
	for _, address := range []string{"example.test:443", "not-an-address", ""} {
		if err := RefuseNonPublic("tcp", address, nil); err == nil {
			t.Errorf("%q was admitted, and an address this hook cannot read is one nothing checked", address)
		}
	}
}

// The point of the whole file: a client built here refuses a request to an
// internal address, through the transport rather than by anything the caller
// remembered to do.
func TestTheOutboundClientRefusesAnInternalAddress(t *testing.T) {
	// A real listener on loopback, so the refusal is the guard's and not a
	// connection that would have failed anyway.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := OutboundClient().Do(request)
	if err == nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Errorf("closing the body of a response that should not exist: %v", closeErr)
		}
		t.Fatalf("the outbound client reached %s — a unit dialling a member-supplied host would reach the "+
			"installation's own network", server.URL)
	}
	if !strings.Contains(err.Error(), "refusing to dial non-public address") {
		t.Errorf("the request failed for the wrong reason: %v", err)
	}
}

// And it is not refusing everything: a client that never connects would pass
// the test above while being useless.
func TestTheOutboundClientStillDialsARoutableAddress(t *testing.T) {
	// No network in a unit lane, so the proof is the DIALER's decision rather
	// than a completed request: the control hook is what stands between the
	// client and the address, and it admits this one.
	dialer := &net.Dialer{Control: RefuseNonPublic}
	if dialer.Control == nil {
		t.Fatal("the published hook is nil")
	}
	if err := dialer.Control("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("a routable address was refused: %v", err)
	}
	if OutboundTransport().DialContext == nil {
		t.Error("the transport dials through the stdlib default, so the guard is not in the path")
	}
}

// A fresh transport per call, because a shared one pools connections and two
// units sharing them would share a pool sized for neither.
func TestEachOutboundTransportIsItsOwn(t *testing.T) {
	first, second := OutboundTransport(), OutboundTransport()
	if first == second {
		t.Error("two calls answered one transport, so units would share a connection pool")
	}
	// And it does not hand out the stdlib's, which nothing else in the process
	// could then dial through unguarded.
	if first == http.DefaultTransport {
		t.Error("the guarded transport IS the process default — the guard would be everywhere or nowhere")
	}
}

// A proxy turns the guard inside out: the dial goes to the proxy's own
// address, which is public and admitted, and the proxy is then asked to
// CONNECT to the target this side never sees. The stdlib default the clone
// inherits is ProxyFromEnvironment, so this is one field away on any
// deployment with HTTPS_PROXY set.
func TestTheGuardedTransportRoutesThroughNoProxy(t *testing.T) {
	if OutboundTransport().Proxy != nil {
		t.Error("the guarded transport carries a proxy — every address the dial control refuses is " +
			"reachable through it, because the proxy is what connects to the target and the dial only " +
			"ever sees the proxy")
	}
	// And the inherited default is what it would have been, so this test is
	// asserting a decision rather than restating a zero value.
	if base, ok := http.DefaultTransport.(*http.Transport); !ok || base.Proxy == nil {
		t.Skip("the stdlib default carries no proxy in this build, so nothing was inherited to drop")
	}
}
