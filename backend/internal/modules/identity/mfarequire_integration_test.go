// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Require-MFA confinement: while the installation requires a second factor, a
// member who holds none is flagged must-enrol at admission — the same mechanism
// a forced password change uses — and the flag clears the moment they confirm a
// factor. An installation that requires nothing pays nothing for the check.

import (
	"context"
	"testing"
)

func (e *revocationEnv) requireMFAOn() {
	e.svc.requireMFA = func(context.Context) (bool, error) { return true, nil }
}

func TestRequireMFAFlagsAMemberWithNoFactorAndClearsOnEnrolment(t *testing.T) {
	e := setupRevocationEnv(t, "require-mfa")
	e.withVault()
	e.requireMFAOn()

	// The member has no factor, so the password alone still opens a session —
	// but admission flags it must-enrol.
	_, token, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	id, err := e.svc.Authenticate(e.wsOnlyCtx(), token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if !id.MustEnrolMFA {
		t.Fatal("a member with no factor under require-MFA is not flagged must-enrol")
	}

	// Enrol and confirm a factor; the same still-valid session now clears — it
	// is the one the confirm names as its own, so the session cutover spares it.
	ctx := e.asMember()
	secret, err := e.svc.StartTOTPEnrolment(ctx)
	if err != nil {
		t.Fatalf("start enrolment: %v", err)
	}
	code, err := totpCodeAt(secret, e.svc.now(), totpDigits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ConfirmTOTP(ctx, code, hashToken(token)); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	cleared, err := e.svc.Authenticate(e.wsOnlyCtx(), token)
	if err != nil {
		t.Fatalf("re-authenticate: %v", err)
	}
	if cleared.MustEnrolMFA {
		t.Error("must-enrol is still set after the member confirmed a factor")
	}
}

func TestAVaultlessServiceNeverChallengesConfinesOrEnrols(t *testing.T) {
	// The posture the composition root leaves when a vault exists but the
	// challenge-signing key is short or absent: the identity service gets NO
	// vault (compose couples the two), and everything MFA must then degrade to
	// "not composed" rather than strand members. The member here confirmed a
	// factor under a working configuration before the key broke.
	e := setupRevocationEnv(t, "mfa-vaultless")
	e.withVault()
	e.enrolMemberMFA(t)

	bare := NewServiceFor(e.svc.db)
	bare.requireMFA = func(context.Context) (bool, error) { return true, nil }

	// Password login never answers the 202 challenge state: a challenge this
	// service could not verify would be a lockout, not a factor.
	_, token, err := bare.Login(e.wsOnlyCtx(), e.member.Email, memberPassword)
	if err != nil {
		t.Fatalf("login on a vaultless service: %v", err)
	}
	if token == "" {
		t.Fatal("no session from the password although no challenge can complete")
	}

	// The require-MFA policy cannot confine while enrolment is impossible.
	id, err := bare.Authenticate(e.wsOnlyCtx(), token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if id.MustEnrolMFA {
		t.Error("a vaultless service confined the member to enrolment routes that can only refuse")
	}

	// And enrolment itself stays the not-composed refusal.
	if _, err := bare.StartTOTPEnrolment(e.asMember()); err == nil {
		t.Error("a vaultless service accepted a TOTP enrolment; the secret had nowhere safe to live")
	}
}

func TestRequireMFAOffLeavesTheMemberUnconfined(t *testing.T) {
	e := setupRevocationEnv(t, "require-mfa-off")
	e.withVault()
	// requireMFA left nil: no enforcement, and no per-request enrolment check.
	_, token, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	id, err := e.svc.Authenticate(e.wsOnlyCtx(), token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if id.MustEnrolMFA {
		t.Error("must-enrol is set with require-MFA off")
	}
}
