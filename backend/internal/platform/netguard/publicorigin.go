// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package netguard

// Whether the origin in an outgoing link can be opened by the person who
// receives it.
//
// This is the inverse of the rule a fetch source answers to. A fetch
// origin may be loopback or private — that is an address THIS process
// dials. A public origin is an address somebody else's mail client dials,
// and "localhost" there means their computer, not ours. The product sent
// a real message whose unsubscribe links pointed at http://localhost:8080;
// nothing refused it, because nothing had been asked to.
//
// The check is syntactic and says so. It refuses the forms that CANNOT
// work — cleartext, loopback, private and other non-public literals — and
// admits a hostname without resolving it, because resolving here would
// make a boot decision depend on DNS at the moment of the check. So it
// proves an origin is not obviously broken; it never proves one is
// reachable. The probe on the Connections screen is what reports that.

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// ErrOriginNotPublic is the class: a configured public origin that a
// recipient could not open.
var ErrOriginNotPublic = errors.New("public origin is not reachable by a recipient")

// RequirePublicOrigin holds a configured public origin to something a
// RECIPIENT can open.
//
// It asks BOTH halves — that the value is a bare origin at all, and that
// the origin is publicly reachable — because the sending callers reach
// only this function. An earlier version delegated the shape half to
// cmd/api's own validator on the grounds that it was stricter; that
// validator runs only when the MCP connector is enabled, so a sending
// deployment with MCP off was left holding nothing, and
// "https://user:secret@example.com" would have gone into every emailed
// link.
//
// Under a development or test posture only the shape is held, so the dev
// stack's http://localhost default keeps working. Any other posture —
// including an unrecognised one, which runtimeenv parses as production —
// gets the full rule, so the carve-out fails closed.
func RequirePublicOrigin(label, raw string, env runtimeenv.Environment) error {
	parsed, err := bareOrigin(label, raw)
	if err != nil {
		return err
	}
	if env == runtimeenv.Development || env == runtimeenv.Test {
		return nil
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: %s is cleartext (%s); a link in somebody's mailbox must be https",
			ErrOriginNotPublic, label, parsed.Scheme)
	}
	return publiclyReachableHost(label, parsed.Hostname())
}

// bareOrigin holds the value to a scheme and a host and nothing else.
//
// Every refusal here is deliberately silent about the value: an origin
// carrying userinfo is carrying a credential, and echoing it would copy
// that credential into the boot log the error lands in.
func bareOrigin(label, raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%w: %s names no origin", ErrOriginNotPublic, label)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s is not a URL", ErrOriginNotPublic, label)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%w: %s carries userinfo, which would be published in every emailed "+
			"link (value withheld: it may contain a credential)", ErrOriginNotPublic, label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: %s needs an http or https scheme", ErrOriginNotPublic, label)
	}
	// Hostname(), not Host: a Host of ":8443" is a non-empty authority that
	// names no host at all, and "https:/example.com" parses to an OPAQUE
	// path with no authority — an https scheme with nothing to dial.
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("%w: %s names no host", ErrOriginNotPublic, label)
	}
	// A URL is derived by APPENDING to this value, so anything after the
	// host swallows what follows it: a fragment makes every appended path
	// part of the fragment, and a query makes it part of the query.
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("%w: %s must be scheme and host only; a path here produces links nothing resolves",
			ErrOriginNotPublic, label)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: %s must be scheme and host only, with no query or fragment",
			ErrOriginNotPublic, label)
	}
	return parsed, nil
}

// publiclyReachableHost refuses the hosts that mean "not from out there".
func publiclyReachableHost(label, host string) error {
	// Lowercased and stripped of a trailing root dot before comparing:
	// "LOCALHOST" and "localhost." are the same name to a resolver, and a
	// check that missed either would refuse the spelling an operator is
	// unlikely to type while admitting the ones they might.
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if name == "localhost" || strings.HasSuffix(name, ".localhost") {
		return fmt.Errorf("%w: %s is %q, which means the RECIPIENT's own computer",
			ErrOriginNotPublic, label, host)
	}
	// A zone (%eth0) is only meaningful on this machine, so an address
	// carrying one can never be somebody else's route in. net.ParseIP
	// rejects the zoned form outright, which would otherwise read as "not
	// an IP" and fall through as if it were a hostname.
	addr, zone, hasZone := strings.Cut(name, "%")
	if hasZone && zone != "" && net.ParseIP(addr) != nil {
		return fmt.Errorf("%w: %s is %q, an address scoped to one machine's own interface",
			ErrOriginNotPublic, label, host)
	}
	if ip := net.ParseIP(name); ip != nil && !PublicIP(ip) {
		return fmt.Errorf("%w: %s is %q, an address that is not reachable from outside this network",
			ErrOriginNotPublic, label, host)
	}
	return nil
}
