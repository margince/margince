// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestMFAChallengeRoundTripCarriesSubjectAndFreshNonce(t *testing.T) {
	signer := newMFAChallengeSigner([]byte(testStateKey))

	token, err := signer.Sign("user-123", 5*time.Minute)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	sub, jti, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if sub != "user-123" {
		t.Errorf("subject = %q, want the member the challenge was minted for", sub)
	}
	if jti == "" {
		t.Fatal("the challenge carries no nonce; it could never be made single-use")
	}

	// A second challenge for the same member gets its own nonce — the nonce is
	// what makes each challenge one login, so two must never share one.
	again, err := signer.Sign("user-123", 5*time.Minute)
	if err != nil {
		t.Fatalf("second Sign: %v", err)
	}
	_, jti2, err := signer.Verify(again)
	if err != nil {
		t.Fatalf("second Verify: %v", err)
	}
	if jti2 == jti {
		t.Error("two challenges share one nonce; spending either spends both")
	}
}

func TestMFAChallengeRefusesTamperAndExpiry(t *testing.T) {
	signer := newMFAChallengeSigner([]byte(testStateKey))
	token, err := signer.Sign("user-123", 5*time.Minute)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if _, _, err := signer.Verify(token + "x"); err == nil {
		t.Error("a tampered signature verified")
	}
	payload, _, ok := strings.Cut(token, ".")
	if !ok {
		t.Fatal("the token has no signature part")
	}
	if _, _, err := signer.Verify(payload + "x." + "forged"); err == nil {
		t.Error("a forged token verified")
	}

	// The clock is a field precisely so expiry is asserted, not slept for.
	stale := signer
	stale.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if _, _, err := stale.Verify(token); err == nil {
		t.Error("an expired challenge verified")
	}
}

func TestMFAEnrolmentArmsOnlyWithBothVaultAndUsableKey(t *testing.T) {
	// Option order is not a contract, so the coupling must hold both ways round.
	orders := map[string][]Option{
		"vault-then-signer": {WithKeyvault(fakeVault{}), WithMFAChallengeSigner(testStateKey)},
		"signer-then-vault": {WithMFAChallengeSigner(testStateKey), WithKeyvault(fakeVault{})},
	}
	for name, opts := range orders {
		s := &Server{log: slog.Default()}
		for _, opt := range opts {
			opt(s, nil)
		}
		if s.mfaSigner == nil {
			t.Errorf("%s: a usable key beside a vault left the challenge signer unarmed", name)
		}
	}
}

func TestAShortKeyBesideAVaultDisarmsMFAAndSaysSoAtError(t *testing.T) {
	var logged bytes.Buffer
	s := &Server{log: slog.New(slog.NewTextHandler(&logged, nil))}
	WithKeyvault(fakeVault{})(s, nil)
	WithMFAChallengeSigner("short")(s, nil)

	if s.mfaSigner != nil {
		t.Fatal("a short key still armed the challenge signer; its tokens would never verify")
	}
	if !s.mfaSignerUnusable {
		t.Fatal("the rejected key was not recorded, so the vault option could never report it")
	}
	if !strings.Contains(logged.String(), "level=ERROR") ||
		!strings.Contains(logged.String(), "MARGINCE_CONNECTOR_STATE_KEY") {
		t.Errorf("the misconfiguration was not reported at ERROR naming the key to fix; log: %s", logged.String())
	}

	// The other order reports through the vault option instead — whichever half
	// arrives second holds the whole picture.
	logged.Reset()
	s2 := &Server{log: slog.New(slog.NewTextHandler(&logged, nil))}
	WithMFAChallengeSigner("short")(s2, nil)
	WithKeyvault(fakeVault{})(s2, nil)
	if s2.mfaSigner != nil {
		t.Fatal("a short key still armed the challenge signer in the reversed order")
	}
	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("the reversed order lost the ERROR report; log: %s", logged.String())
	}
}
