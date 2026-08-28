// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The client a unit dials its provider through.
//
// ReservedNets already publishes WHAT the installation refuses. What every
// unit then had to write for itself is the part that makes the list do
// anything: a dialer whose Control hook runs the check AFTER resolution, on
// the concrete address it is about to connect to. That is a small amount of
// code and exactly the kind that is wrong in a way nobody notices — a unit
// that checks the URL's hostname before dialling has written a pre-flight
// lookup, not a guard, and the attacker who cares controls the DNS between
// the two.
//
// So the guard is DELIVERED rather than described. A unit asks for a client
// and gets the policy inside it, and a range added to the published list
// reaches every unit at once instead of every unit that remembered.
//
// WHAT IT GUARANTEES: every address the client connects to is checked after
// resolution, so a hostname that resolves into a reserved range is refused
// even though its text looked public, and one that REBINDS between the
// resolution and the connect is refused at the connect.
//
// WHAT IT DOES NOT: it is not an allowlist, not a proxy, and not a rate limit.
// It refuses addresses the installation refuses; the method, the headers, the
// body and the deadline are the unit's own. Redirects are followed by the
// stdlib as usual — and are dialled through the same Control, which is the
// half a hand-written guard most often misses, because the refusal has to
// happen on the address the redirect names rather than on the one the caller
// typed.
//
// WHAT A UNIT MUST NOT DO is assemble its own. TestNoUnitDialsAroundTheInstallationsEgressPolicy
// refuses a unit that writes its own denylist or its own dialer control: both
// are copies that stop being equal to the published list the moment it moves.

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// outboundDialTimeout bounds the CONNECT, not the request.
//
// A request's own deadline is the caller's business and belongs on its
// context; what this bounds is the case a context deadline handles badly — a
// host that accepts nothing and never answers, where without a dial timeout
// every worker that touches it waits for the platform's own TCP timeout,
// minutes later. 10s is far longer than any reachable host needs and far
// shorter than that.
const outboundDialTimeout = 10 * time.Second

// RefuseNonPublic is a net.Dialer.Control hook that refuses any non-public
// address. Published for the unit that needs its own client settings — a
// custom TLS config, a different proxy — and can therefore not use
// OutboundClient as it stands: wire this as Control on the dialer and the
// guard is the same one.
//
// It runs on the concrete IP the dialer is about to connect to, which is the
// only place the check holds.
func RefuseNonPublic(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("extension: unparseable dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// The dialer hands a literal here, always. A name at this point means
		// the caller wired this hook somewhere it does not belong, and
		// admitting it would be admitting an unchecked address.
		return fmt.Errorf("extension: dial address %q is not a literal IP", host)
	}
	if !publicIP(ip) {
		return fmt.Errorf("extension: refusing to dial non-public address %s", host)
	}
	return nil
}

// OutboundTransport answers a transport whose dials the egress policy admits.
//
// A fresh one per call, and the reason is connection pooling: a shared
// transport keeps idle connections, and two units sharing them would share a
// pool sized for neither. A unit builds one and holds it — the same rule any
// http.Transport carries — rather than building one per request.
func OutboundTransport() *http.Transport {
	transport, _ := http.DefaultTransport.(*http.Transport)
	guarded := transport.Clone()
	dialer := &net.Dialer{Timeout: outboundDialTimeout, KeepAlive: 30 * time.Second, Control: RefuseNonPublic}
	guarded.DialContext = dialer.DialContext
	return guarded
}

// OutboundClient answers an http.Client whose dials the egress policy admits.
//
// No client Timeout is set, deliberately. A client-wide timeout bounds the
// whole exchange including the body read, so a unit streaming a large export
// would have it cut mid-download by a number this package chose. The deadline
// belongs on the caller's context, where it is the unit's decision and applies
// to the one request that needs it.
func OutboundClient() *http.Client {
	return &http.Client{Transport: OutboundTransport()}
}
