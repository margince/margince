// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The signing scheme a session-less POST into the installation carries. A web
// form's public post uses it, and it is published so a sender outside the tree
// can build the same signature.
//
//margince:extension-surface

package extension

import (
	"fmt"
	"regexp"
	"time"
)

// The header names a signed post carries. The signature covers all three, and
// the address posted to as well (see SigningPayload), so none of them can be
// edited without invalidating it.
const (
	// InboundHeaderTimestamp carries unix SECONDS, as decimal digits.
	InboundHeaderTimestamp = "X-Margince-Timestamp"

	// InboundHeaderNonce carries a caller-chosen unique value, hex-encoded. It
	// makes a correctly signed post land once: a second one with the same nonce
	// lands nothing.
	InboundHeaderNonce = "X-Margince-Nonce"

	// InboundHeaderSignature carries `sha256=<hex>`, the HMAC-SHA256 over
	// what SigningPayload builds.
	InboundHeaderSignature = "X-Margince-Signature"
)

// SigningScope names which exchange a signature belongs to. It is the first line
// of every signed payload, so one exchange's signature is never valid in another.
type SigningScope string

// ScopeInbound is a post arriving at the installation.
const ScopeInbound SigningScope = "margince-inbound-v1"

// SigningPayload is the exact material a signature covers. Compute the MAC over
// this and nothing else.
//
// The encoding is unambiguous. Every field before the body comes from an
// alphabet with no newline in it, so the first five newlines are the five
// separators. A plain join is ambiguous: moving the boundary makes a new nonce
// over the same material. One captured post could then replay once per separator.
func SigningPayload(scope SigningScope, slug, ref string, at time.Time, nonce string, body []byte) []byte {
	head := fmt.Sprintf("%s\n%s\n%s\n%d\n%s\n", scope, slug, ref, at.Unix(), nonce)
	payload := make([]byte, 0, len(head)+len(body))
	payload = append(payload, head...)
	return append(payload, body...)
}

// MaxInboundNonce bounds the replay key a caller may present. A receiver stores
// the nonce, so an unbounded one is an unbounded write.
const MaxInboundNonce = 128

// inboundNonceGrammar is enforced because SigningPayload's unambiguity rests on
// it: a nonce that could hold the separator could absorb the head of the body.
var inboundNonceGrammar = regexp.MustCompile(`^[0-9a-fA-F]+$`)

// ValidInboundNonce reports whether a presented replay key is one a receiver accepts.
func ValidInboundNonce(nonce string) bool {
	return len(nonce) <= MaxInboundNonce && inboundNonceGrammar.MatchString(nonce)
}
