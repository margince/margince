// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLoginStateSignerRoundTrip(t *testing.T) {
	signer := newLoginStateSigner([]byte("0123456789012345678901234567890123"))
	now := time.Unix(1_700_000_000, 0)
	for _, st := range []loginState{
		{Provider: "google", ClientID: "cid", Nonce: "abc123", CodeVerifier: "verifier-xyz"},
		{Provider: "google", ClientID: "cid", Nonce: "abc123", CodeVerifier: "verifier-xyz", ReturnToConsent: true},
	} {
		got, err := signer.verify(signer.sign(st, now.Add(10*time.Minute)), now)
		if err != nil {
			t.Fatalf("verify: %v", err)
		}
		if got != st {
			t.Fatalf("got %+v, want %+v", got, st)
		}
	}
}

// The consent marker decides where a session lands. A browser that sets it in
// its own cookie is refused like any other edit to the state.
func TestLoginStateSignerRefusesAnAddedConsentMarker(t *testing.T) {
	signer := newLoginStateSigner([]byte("0123456789012345678901234567890123"))
	now := time.Unix(1_700_000_000, 0)
	token := signer.sign(loginState{Provider: "google", Nonce: "n"}, now.Add(time.Minute))
	enc, mac, _ := strings.Cut(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	fields["rc"] = true
	edited, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	forged := base64.RawURLEncoding.EncodeToString(edited) + "." + mac
	if _, err := signer.verify(forged, now); err == nil {
		t.Fatal("a state with an added consent marker verified")
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

// TestLoginStateSignerRejectsAConnectorMintedToken closes the cross-parse gap
// a shared key opens: cmd/api/googlesignin.go signs with the SAME
// --connector-state-key stateSigner uses, and neither wire struct's
// non-omitempty fields are required at unmarshal time, so a validly-signed
// connectorstate.go token would otherwise decode into a loginState with
// empty Nonce/CodeVerifier and pass the MAC check. The `t` field this test
// pins is what makes the two token kinds refuse to parse as each other.
func TestLoginStateSignerRejectsAConnectorMintedToken(t *testing.T) {
	key := []byte("0123456789012345678901234567890123")
	now := time.Unix(1_700_000_000, 0)

	connectorToken := newStateSigner(key).sign(connectState{
		Provider: "google", Nonce: "n",
	}, now.Add(10*time.Minute))

	if _, err := newLoginStateSigner(key).verify(connectorToken, now); err == nil {
		t.Fatal("expected a connector-flow token to be refused by the login-flow signer")
	}
}
