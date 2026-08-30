// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"
)

func TestLoginStateSignerRoundTrip(t *testing.T) {
	signer := newLoginStateSigner([]byte("0123456789012345678901234567890123"))
	now := time.Unix(1_700_000_000, 0)
	st := loginState{Provider: "google", Nonce: "abc123", CodeVerifier: "verifier-xyz"}
	token := signer.sign(st, now.Add(10*time.Minute))

	got, err := signer.verify(token, now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Provider != st.Provider || got.Nonce != st.Nonce || got.CodeVerifier != st.CodeVerifier {
		t.Fatalf("got %+v, want %+v", got, st)
	}
}

func TestLoginStateSignerRejectsExpired(t *testing.T) {
	signer := newLoginStateSigner([]byte("0123456789012345678901234567890123"))
	now := time.Unix(1_700_000_000, 0)
	token := signer.sign(loginState{Provider: "google", Nonce: "n"}, now.Add(-time.Minute))
	if _, err := signer.verify(token, now); err == nil {
		t.Fatal("expected expiry rejection")
	}
}

func TestLoginStateSignerRejectsTamperedPayload(t *testing.T) {
	signer := newLoginStateSigner([]byte("0123456789012345678901234567890123"))
	now := time.Unix(1_700_000_000, 0)
	token := signer.sign(loginState{Provider: "google", Nonce: "n"}, now.Add(time.Minute))
	// Flip a character well inside the payload segment (not the trailing
	// base64 character of either segment, whose few significant bits can
	// coincidentally survive a single-character substitution).
	flip := byte('A')
	if token[5] == 'A' {
		flip = 'B'
	}
	tampered := token[:5] + string(flip) + token[6:]
	if _, err := signer.verify(tampered, now); err == nil {
		t.Fatal("expected signature rejection")
	}
}
