// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Password sessions need a confirmed factor when the policy requires one.
// Provider sessions rely on the provider’s authentication policy.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
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

// With require_mfa on, a Google or Microsoft sign-in counts as the second
// factor. The provider session is never confined or challenged.
//
// A password session is confined while the member holds no factor. Once they
// hold one, the password path asks for it.
func TestRequireMFATrustsTheProviderAndAsksThePasswordPath(t *testing.T) {
	e := setupRevocationEnv(t, "require-mfa-paths")
	e.withVault()
	e.requireMFAOn()

	// No factor yet: the provider session is not confined.
	token, err := e.svc.LoginViaFederatedIdentity(e.wsOnlyCtx(), "google",
		OIDCClaims{Subject: "sub-mfa-paths", Email: e.member.Email})
	if err != nil {
		t.Fatalf("provider sign-in: %v", err)
	}
	id, err := e.svc.Authenticate(e.wsOnlyCtx(), token)
	if err != nil {
		t.Fatalf("authenticate a provider session: %v", err)
	}
	if id.MustEnrolMFA {
		t.Error("a provider session for a member with no factor is confined, want it trusted")
	}

	// No factor yet: the password session is confined to enrolment.
	_, login, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice)
	if err != nil {
		t.Fatalf("password sign-in with no factor: %v", err)
	}
	passwordID, err := e.svc.Authenticate(e.wsOnlyCtx(), login.Token)
	if err != nil {
		t.Fatalf("authenticate a password session: %v", err)
	}
	if !passwordID.MustEnrolMFA {
		t.Error("a password session for a member with no factor is not confined")
	}

	// The member confirms a factor. The password path now challenges, and the
	// provider path still signs them in.
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
	signedIn, err := e.svc.LoginViaFederatedIdentity(e.wsOnlyCtx(), "google",
		OIDCClaims{Subject: "sub-mfa-paths", Email: e.member.Email})
	if err != nil {
		t.Fatalf("provider sign-in with a confirmed factor: %v", err)
	}
	unconfined, err := e.svc.Authenticate(e.wsOnlyCtx(), signedIn)
	if err != nil {
		t.Fatalf("authenticate the provider session: %v", err)
	}
	if unconfined.MustEnrolMFA {
		t.Error("provider session with a confirmed factor is confined, want an unconfined session")
	}
	if unconfined.UserID != e.member.UserID {
		t.Errorf("provider session belongs to %v, want the member %v who confirmed the factor",
			unconfined.UserID, e.member.UserID)
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

func TestProviderSessionsAreExemptFromPasswordMFAConfinement(t *testing.T) {
	for _, provider := range []string{"google", "microsoft"} {
		t.Run(provider, func(t *testing.T) {
			e := setupRevocationEnv(t, "provider-mfa-"+provider)
			e.withVault()
			e.requireMFAOn()
			token, err := e.svc.LoginViaFederatedIdentity(e.wsOnlyCtx(), provider,
				OIDCClaims{Subject: "provider-member", Email: e.member.Email})
			if err != nil {
				t.Fatal(err)
			}
			id, err := e.svc.Authenticate(e.wsOnlyCtx(), token)
			if err != nil {
				t.Fatal(err)
			}
			if id.MustEnrolMFA {
				t.Fatal("provider session confined to Margince MFA enrolment")
			}
			_, passwordSession, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice)
			if err != nil {
				t.Fatal(err)
			}
			passwordID, err := e.svc.Authenticate(e.wsOnlyCtx(), passwordSession.Token)
			if err != nil {
				t.Fatal(err)
			}
			if !passwordID.MustEnrolMFA {
				t.Fatal("a provider link exempted a password session")
			}
		})
	}
}

func TestASessionWithoutSignInProvenanceKeepsThePasswordMFARule(t *testing.T) {
	e := setupRevocationEnv(t, "legacy-session-mfa")
	e.withVault()
	e.requireMFAOn()
	token, hash, err := mintSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO session (user_id, token_hash, idle_expires_at, expires_at)
  VALUES (@user, @hash, now()+interval '1 hour', now()+interval '1 hour')`, pgx.StrictNamedArgs{"user": e.member.UserID, "hash": hash}); err != nil {
		t.Fatal(err)
	}
	id, err := e.svc.Authenticate(e.wsOnlyCtx(), token)
	if err != nil {
		t.Fatal(err)
	}
	if !id.MustEnrolMFA {
		t.Fatal("an unknown sign-in method bypassed password MFA enrolment")
	}
}
