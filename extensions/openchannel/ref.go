// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The handle that makes one URL per member.
//
// A declared slug is the same for every caller — it is a literal in the unit's
// declaration — so it cannot be what an arriving request is resolved by. The ref
// is the trailing path segment the core hands through untouched, and it is what
// this unit resolves to an owner.
//
// IT IS NOT A CREDENTIAL, and that is worth saying three times because a reader
// who assumes otherwise builds something unsafe on it. It travels in the path,
// so it is written to every access log and every proxy between a sender and
// here, exactly as the slug is. What admits a request is the signature over the
// endpoint's sealed secret. The ref is unguessable only so that two members
// cannot collide and so that a URL is not trivially enumerable — never so that
// holding one means anything.

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/margince/margince/backend/pkg/extension"
)

// refBytes is how much entropy a minted ref carries. Sixteen bytes is 128 bits,
// which is past any collision worth reasoning about across every endpoint an
// installation will ever open, and it encodes to 22 characters — comfortably
// inside the core's bound, so a member's URL stays short enough to read out.
const refBytes = 16

// newEndpointRef mints one.
//
// base64url WITHOUT padding, because the core's published alphabet is
// [A-Za-z0-9_-] and `=` is not in it. Standard base64 would produce `+` and `/`,
// and the second of those would end the path segment.
func newEndpointRef() (string, error) {
	raw := make([]byte, refBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("openchannel: the system random source is unavailable, so no endpoint address can be minted: %w", err)
	}
	ref := base64.RawURLEncoding.EncodeToString(raw)
	// Held against the CORE's own predicate rather than against a copy of its
	// grammar. The edge answers 404 to a path segment it will not route, before
	// this unit's handler is reached at all — so a ref that failed here would be
	// a URL handed to a member that silently never works, and the endpoint would
	// look open while nothing could ever arrive on it.
	if !extension.ValidInboundRef(ref) {
		return "", fmt.Errorf("openchannel: minted an address of %d characters that the inbound edge would not route", len(ref))
	}
	return ref, nil
}
