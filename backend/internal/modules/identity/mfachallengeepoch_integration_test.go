// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A second-factor challenge stands only while the member's password and factor
// are the ones it was issued under. A change in between refuses the challenge,
// as neutrally as a wrong code.

import (
	"errors"
	"testing"
	"time"
)

// passwordChallenge passes the password step and returns the challenge it
// stops at, with a nonce as the handler's signature would carry.
func (e *revocationEnv) passwordChallenge(t *testing.T) MFAChallenge {
	t.Helper()
	_, _, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice)
	var owed *challengeOwed
	if !errors.As(err, &owed) {
		t.Fatalf("password step: err = %v, want a challenge", err)
	}
	if owed.challenge.Epoch == "" {
		t.Fatal("the challenge carries no credential epoch")
	}
	issued := owed.challenge
	issued.Jti = freshJti()
	return issued
}

func TestAChallengeIsRefusedOnceTheCredentialsChange(t *testing.T) {
	base := time.Unix(1_760_000_000, 0)
	changes := map[string]func(e *revocationEnv, t *testing.T, secret *string){
		"password changed": func(e *revocationEnv, t *testing.T, _ *string) {
			if _, err := e.svc.ChangePassword(e.asMember(), memberPassword, "a changed password!"); err != nil {
				t.Fatalf("change password: %v", err)
			}
		},
		"password reset": func(e *revocationEnv, t *testing.T, _ *string) {
			raw, _, err := e.svc.IssuePasswordLink(e.wsCtx(e.admin), e.admin, e.member.UserID)
			if err != nil {
				t.Fatalf("issue link: %v", err)
			}
			if err := e.svc.RedeemPasswordReset(e.wsCtx(e.admin), raw, "a reset password!"); err != nil {
				t.Fatalf("redeem: %v", err)
			}
		},
		"factor removed and enrolled again": func(e *revocationEnv, t *testing.T, secret *string) {
			e.atInstant(base.Add(totpStep))
			code, _ := totpCodeAt(*secret, e.svc.now(), totpDigits)
			if err := e.svc.DisableMFA(e.asMember(), code); err != nil {
				t.Fatalf("disable: %v", err)
			}
			*secret = e.enrolMemberMFA(t)
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			e := setupRevocationEnv(t, "mfa-epoch")
			e.withVault()
			e.atInstant(base)
			secret := e.enrolMemberMFA(t)
			issued := e.passwordChallenge(t)

			change(e, t, &secret)

			e.atInstant(base.Add(3 * totpStep))
			code, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
			if _, token, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, issued, code); !errors.Is(err, errBadMFACode) || token != "" {
				t.Fatalf("completion after the change: token %q, err = %v, want %v", token, err, errBadMFACode)
			}
		})
	}
}

// The unchanged case, so the refusals above are about the change.
func TestAChallengeStandsWhileTheCredentialsDoNot(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-epoch-kept")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
	secret := e.enrolMemberMFA(t)
	issued := e.passwordChallenge(t)
	e.atInstant(base.Add(totpStep))
	code, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, issued, code); err != nil {
		t.Fatalf("completion: %v", err)
	}
	unkeyed := issued
	unkeyed.Jti, unkeyed.Epoch = freshJti(), ""
	e.atInstant(base.Add(2 * totpStep))
	next, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, unkeyed, next); !errors.Is(err, errBadMFACode) {
		t.Fatalf("a challenge with no epoch: err = %v, want %v", err, errBadMFACode)
	}
}
