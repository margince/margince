// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// EncodeSigned renders a position as an opaque token the client cannot edit:
// the EncodeOpaque body, a dot, and an HMAC-SHA256 of that body under key.
func EncodeSigned[T any](position T, key []byte) (string, error) {
	body, err := EncodeOpaque(position)
	if err != nil {
		return "", err
	}
	return body + "." + signedCursorMAC(body, key), nil
}

// DecodeSigned reads a token EncodeSigned minted under key. A token with no
// signature, a wrong one or an edited body is refused as MalformedCursorError,
// the same answer a token this server never minted gets.
func DecodeSigned[T any](token string, key []byte) (T, error) {
	body, mac, signed := strings.Cut(token, ".")
	if !signed || !hmac.Equal([]byte(mac), []byte(signedCursorMAC(body, key))) {
		var zero T
		return zero, &MalformedCursorError{}
	}
	return DecodeOpaque[T](body)
}

func signedCursorMAC(body string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}
