// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package bearer mints a capability token and the digest a database stores in
// its place.
//
// The rule it exists to keep is one rule: the raw token never reaches the
// database. Every bearer capability in this tree already obeys it — sessions,
// invites, OAuth refresh, confirmation links — and each spelled the mint and
// the hash itself. A second spelling is where the two drift, and the way they
// drift is a table that stores a token somebody can read.
//
// Storing the digest means a database dump, a log line or a backup carries
// nothing that opens anything. It also means the token cannot be shown twice:
// there is nothing to show it FROM, which is why every caller returns the raw
// value once at creation and never again.
package bearer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenBytes is how much entropy a capability carries.
//
// 256 bits. A share link is a URL people paste into mail and chat, so it will
// be seen by more parties than the one it was issued to, and its only defence
// is being unguessable.
const tokenBytes = 32

// Mint returns the raw token to hand out once, and the digest to store.
//
// Both together, from one call, because a caller that hashed separately could
// store a digest of something other than what it handed out — and the failure
// is silent until somebody tries to use the link.
func Mint() (raw, digest string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("bearer: minting a token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, Digest(raw), nil
}

// Digest is what the database stores in a token's place.
//
// Plain SHA-256 rather than a password hash, and deliberately: a token carries
// 256 bits of entropy from a CSPRNG, so there is no dictionary to slow down.
// The work factor a password needs would buy nothing here and would cost every
// lookup.
func Digest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
