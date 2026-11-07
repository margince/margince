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
	"errors"
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
	_, login, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	id, err := e.svc.Authenticate(e.wsOnlyCtx(), login.Token)
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
	if _, err := e.svc.ConfirmTOTP(ctx, code, hashToken(login.Token)); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	cleared, err := e.svc.Authenticate(e.wsOnlyCtx(), login.Token)
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
	_, login, err := bare.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice)
	if err != nil {
		t.Fatalf("login on a vaultless service: %v", err)
	}
	if login.Token == "" {
		t.Fatal("no session from the password although no challenge can complete")
	}

	// The require-MFA policy cannot confine while enrolment is impossible.
	id, err := bare.Authenticate(e.wsOnlyCtx(), login.Token)
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

// One installation setting, two sign-in paths, two answers. This pins what the
// code does today, which the two paths do not agree on.
//
// A member who has confirmed a factor is challenged on the password path. A
// provider signs the same member straight in.
//
// A member who has not confirmed one is confined by require_mfa, whatever
// opened the session. That includes a provider sign-in, which will never ask
// for the factor they are told to enrol.
//
// Which reading is right is open on #7655. Either the provider counts as the
// second factor, or a provider sign-in challenges as the password path does.
// This test is here so settling it means editing a test that states the reading
// it replaced.
func TestRequireMFAAsksOnlyThePasswordPathWhileConfiningBoth(t *testing.T) {
	e := setupRevocationEnv(t, "require-mfa-paths")
	e.withVault()
	e.requireMFAOn()

	// Confined with no factor, through a provider sign-in that cannot ask for
	// one. The member is admitted and then held at the enrolment routes.
	token, err := e.svc.LoginViaFederatedIdentity(e.wsOnlyCtx(), "google",
		OIDCClaims{Subject: "sub-mfa-paths", Email: e.member.Email})
	if err != nil {
		t.Fatalf("provider sign-in: %v", err)
	}
	id, err := e.svc.Authenticate(e.wsOnlyCtx(), token)
	if err != nil {
		t.Fatalf("authenticate a provider session: %v", err)
	}
	if !id.MustEnrolMFA {
		t.Error("a provider session for a member with no factor is not confined. If #7655 chose to " +
			"exempt provider sessions, this test states the reading it replaced")
	}

	// Now the same member holds a confirmed factor. The password path turns
	// into a challenge, and the provider path still does not.
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

	if _, _, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice); !errors.Is(err, errMFARequired) {
		t.Errorf("password sign-in with a confirmed factor: err = %v, want %v", err, errMFARequired)
	}
	// The provider path opens a session for the same confirmed factor rather
	// than challenging it. A refusal here is the other reading, not a failure
	// of this environment, so it names where that was decided.
	if _, err := e.svc.LoginViaFederatedIdentity(e.wsOnlyCtx(), "google",
		OIDCClaims{Subject: "sub-mfa-paths", Email: e.member.Email}); err != nil {
		t.Errorf("provider sign-in with a confirmed factor: %v. If #7655 chose to challenge the "+
			"provider path, this test states the reading it replaced", err)
	}
}

func TestRequireMFAOffLeavesTheMemberUnconfined(t *testing.T) {
	e := setupRevocationEnv(t, "require-mfa-off")
	e.withVault()
	// requireMFA left nil: no enforcement, and no per-request enrolment check.
	_, login, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	id, err := e.svc.Authenticate(e.wsOnlyCtx(), login.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if id.MustEnrolMFA {
		t.Error("must-enrol is set with require-MFA off")
	}
}
