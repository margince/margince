// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// abcDigest is the published SHA-256 of "abc". A stored hash that moved away
// from it would stop matching the sessions and links already issued.
const abcDigest = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

func TestATextColumnStoresTheDigestAsLowerCaseHex(t *testing.T) {
	if got := TokenHash("abc"); got != abcDigest {
		t.Fatalf("TokenHash(abc) = %q, want %q", got, abcDigest)
	}
}

func TestAByteaColumnStoresTheThirtyTwoDigestBytes(t *testing.T) {
	want, err := hex.DecodeString(abcDigest)
	if err != nil {
		t.Fatal(err)
	}
	if got := TokenHashBytes("abc"); !bytes.Equal(got, want) {
		t.Fatalf("TokenHashBytes(abc) = %x, want %x", got, want)
	}
}

func TestAMintedTokenIsStoredAsItsOwnHash(t *testing.T) {
	raw, hash, err := MintToken()
	if err != nil {
		t.Fatal(err)
	}
	entropy, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(entropy) != tokenBytes {
		t.Fatalf("a minted token decodes to %d bytes (%v), want %d", len(entropy), err, tokenBytes)
	}
	if hash != TokenHash(raw) {
		t.Fatalf("the minted hash %q is not the hash of the token handed out", hash)
	}
}
