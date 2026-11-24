// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenBytes is 256 bits: a token is a link pasted into mail and chat, and
// being unguessable is its one defence.
const tokenBytes = 32

// MintToken returns the raw token to hand out once and the hash a text column
// stores. One call makes both, so the stored hash is of the token handed out.
func MintToken() (raw, hash string, err error) {
	entropy := make([]byte, tokenBytes)
	if _, err := rand.Read(entropy); err != nil {
		return "", "", fmt.Errorf("storekit: minting a token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(entropy)
	return raw, TokenHash(raw), nil
}

// TokenHash is what a text token_hash column stores in a token's place: the
// SHA-256 as 64 lower-case hex characters. A random token has no dictionary,
// so a slow password hash would cost each lookup and buy nothing.
func TokenHash(raw string) string {
	return hex.EncodeToString(TokenHashBytes(raw))
}

// TokenHashBytes is what a bytea token_hash column stores: the 32 digest bytes.
func TokenHashBytes(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
