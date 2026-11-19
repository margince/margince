// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"strings"
	"testing"
)

func TestASignedTokenRoundTripsAndRefusesEditsAndOtherKeys(t *testing.T) {
	type position struct{ N int }
	key := []byte("0123456789abcdef0123456789abcdef")
	token, err := EncodeSigned(position{N: 4}, key)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeSigned[position](token, key); err != nil || got.N != 4 {
		t.Fatalf("round trip gave %+v, %v", got, err)
	}
	other, err := EncodeOpaque(position{N: 9})
	if err != nil {
		t.Fatal(err)
	}
	_, mac, _ := strings.Cut(token, ".")
	for name, bad := range map[string]string{
		"an edited body":  other + "." + mac,
		"no signature":    other,
		"another key":     token,
		"an empty string": "",
	} {
		useKey := key
		if name == "another key" {
			useKey = []byte("ffffffffffffffffffffffffffffffff")
		}
		if _, err := DecodeSigned[position](bad, useKey); !errors.As(err, new(*MalformedCursorError)) {
			t.Errorf("%s: got %v, want MalformedCursorError", name, err)
		}
	}
}
