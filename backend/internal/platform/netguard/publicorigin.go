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

// RequirePublicOrigin holds a configured public origin to what a
// RECIPIENT can open.
//
// Shape — scheme, host, no path or userinfo — is somebody else's job:
// cmd/api's validateBareOrigin already holds it, more strictly than a
// second copy here would, and this adds only the half that one does not
// ask. It takes the value as given and judges reachability alone.
//
// Under a development or test posture nothing is held, so the dev stack's
// http://localhost default keeps working. Any other posture — including
// an unrecognised one, which runtimeenv parses as production — gets the
// full rule, so the carve-out fails closed.
func RequirePublicOrigin(label, raw string, env runtimeenv.Environment) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%w: %s names no origin", ErrOriginNotPublic, label)
	}
	if env == runtimeenv.Development || env == runtimeenv.Test {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %s is not a URL", ErrOriginNotPublic, label)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: %s is cleartext (%s); a link in somebody's mailbox must be https",
			ErrOriginNotPublic, label, parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("%w: %s is %q, which means the RECIPIENT's own computer",
			ErrOriginNotPublic, label, host)
	}
	if ip := net.ParseIP(host); ip != nil && !PublicIP(ip) {
		return fmt.Errorf("%w: %s is %q, an address that is not reachable from outside this network",
			ErrOriginNotPublic, label, host)
	}
	return nil
}
