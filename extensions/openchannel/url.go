// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// Where this connector talks back to, checked against what a member typed.
//
// A SECOND WRITER OF THIS RULE EXISTS IN THE TREE, in a sibling connector's own
// module, and the two do not share a helper because they cannot: a unit is its
// own Go module and a module never imports a sibling. The half that would
// actually be dangerous to keep as two copies — which address ranges may not be
// reached — is not here at all: it belongs to whatever dials the address, and
// it is published as extension.ReservedNets so a unit takes the core's list
// rather than a hand-copy of it.

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// maxURLLength bounds what is stored and later dialed. It is the length every
// mainstream client and proxy handles without truncating, so a URL over it is
// one that would be cut somewhere between here and the far end rather than one
// that merely looks long.
const maxURLLength = 2048

// registrableURL validates an outward address where a person can still read the
// refusal, against what they typed, rather than at the moment something tries
// to dial it.
//
// https only, because whatever this connector sends rides it. A host name
// rather than an address literal, because an address literal is how the
// interesting internal targets are named and no real deployment is reached that
// way. No credentials in the URL, and no fragment: the first would be sent on
// every request, and the second is never transmitted at all, so a member who
// put meaning in one would have it silently dropped.
func registrableURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: registering an address needs one — give the https URL this connector should post to", extension.ErrInvalid)
	}
	if len(trimmed) > maxURLLength {
		return "", fmt.Errorf("%w: that address is %d characters, over the %d-character cap", extension.ErrInvalid, len(trimmed), maxURLLength)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: that address is not a URL", extension.ErrInvalid)
	}
	switch {
	case parsed.Scheme != "https":
		return "", fmt.Errorf("%w: the address must be https — what this connector sends rides it", extension.ErrInvalid)
	case parsed.Host == "":
		return "", fmt.Errorf("%w: the address names no host", extension.ErrInvalid)
	case parsed.User != nil:
		return "", fmt.Errorf("%w: the address carries credentials, which would be sent on every request", extension.ErrInvalid)
	case parsed.Fragment != "":
		return "", fmt.Errorf("%w: the address carries a fragment, which is never transmitted", extension.ErrInvalid)
	case net.ParseIP(hostOnly(parsed.Host)) != nil:
		return "", fmt.Errorf("%w: the address must name a host, not an address", extension.ErrInvalid)
	}
	return parsed.String(), nil
}

// hostOnly drops the port from a host:port, and answers the whole string when
// there is none — which is what net.SplitHostPort reports as an error.
func hostOnly(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
