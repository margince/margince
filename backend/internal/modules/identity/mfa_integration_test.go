// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The MFA store end to end over a real database and a real (in-memory) seal:
// enrol an authenticator, confirm it into an active factor with recovery codes,
// verify both a live code and a one-time recovery code, and tear it down. The
// secret never lands in the row — only its vault ref does — and no accepted
// code, authenticator or recovery, is ever accepted twice.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *revocationEnv) withVault() {
	e.svc.WithVault(keyvault.NewMemory())
}

// atInstant pins the service clock, so which 30-second TOTP step a code lands
// in is a property the test states rather than one the wall clock decides.
func (e *revocationEnv) atInstant(t time.Time) {
	e.svc.now = func() time.Time { return t }
}

func (e *revocationEnv) verifyMemberMFA(t *testing.T, code string) bool {
	t.Helper()
	var ok bool
	err := e.svc.db.Tx(e.wsOnlyCtx(), func(tx pgx.Tx) error {
		var vErr error
		ok, vErr = e.svc.verifyMFA(e.wsOnlyCtx(), tx,
			ids.From[ids.WorkspaceKind](e.member.WorkspaceID.UUID), e.member.UserID, code)
		return vErr
	})
	if err != nil {
		t.Fatalf("verifyMFA: %v", err)
	}
	return ok
}

func TestTOTPEnrolmentConfirmVerifyAndRecovery(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-enrol")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
	ctx := e.asMember()

	secret, err := e.svc.StartTOTPEnrolment(ctx)
	if err != nil {
		t.Fatalf("StartTOTPEnrolment: %v", err)
	}
	if secret == "" {
		t.Fatal("enrolment returned no secret to scan")
	}

	// The secret column holds a vault REF, never the secret itself.
	var storedRef string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT secret_ref FROM user_mfa WHERE user_id = $1`, e.member.UserID).Scan(&storedRef); err != nil {
		t.Fatalf("reading stored ref: %v", err)
	}
	if storedRef == secret || storedRef == "" {
		t.Errorf("the row stored the raw secret instead of a sealed ref: %q", storedRef)
	}

	code, err := totpCodeAt(secret, base, totpDigits)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := e.svc.ConfirmTOTP(ctx, code, "")
	if err != nil {
		t.Fatalf("ConfirmTOTP: %v", err)
	}
	if len(recovery) != recoveryCodeCount {
		t.Fatalf("got %d recovery codes, want %d", len(recovery), recoveryCodeCount)
	}

	status, err := e.svc.MFAEnrolment(ctx)
	if err != nil {
		t.Fatalf("MFAEnrolment: %v", err)
	}
	if !status.Enrolled || !status.Confirmed || status.RecoveryCodesLeft != recoveryCodeCount {
		t.Fatalf("status after confirm = %+v", status)
	}

	// The confirmation already spent its time-step: the same code presented to
	// a login inside the skew window is a replay, not a second proof.
	if e.verifyMemberMFA(t, code) {
		t.Error("the confirmation code verified again at login; an intercepted code replays")
	}

	// One step later a fresh authenticator code verifies — exactly once.
	e.atInstant(base.Add(totpStep))
	fresh, err := totpCodeAt(secret, base.Add(totpStep), totpDigits)
	if err != nil {
		t.Fatal(err)
	}
	if !e.verifyMemberMFA(t, fresh) {
		t.Error("a valid authenticator code did not verify")
	}
	if e.verifyMemberMFA(t, fresh) {
		t.Error("an accepted authenticator code verified a second time; the step guard is not recording")
	}

	// A recovery code verifies once and is then spent.
	if !e.verifyMemberMFA(t, recovery[0]) {
		t.Error("a recovery code did not verify")
	}
	if e.verifyMemberMFA(t, recovery[0]) {
		t.Error("a recovery code verified a second time; it must be single-use")
	}
	if left := statusOf(t, e).RecoveryCodesLeft; left != recoveryCodeCount-1 {
		t.Errorf("recovery codes left = %d, want %d after spending one", left, recoveryCodeCount-1)
	}
}

func TestMFACannotBeReEnrolledWhileConfirmedAndDisableDemandsTheFactor(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-reenrol")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
	ctx := e.asMember()

	secret, err := e.svc.StartTOTPEnrolment(ctx)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code, _ := totpCodeAt(secret, base, totpDigits)
	recovery, err := e.svc.ConfirmTOTP(ctx, code, "")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// A confirmed factor must be disabled before a new one can be started, or a
	// silent re-enrolment would swap the factor under the member.
	if _, err := e.svc.StartTOTPEnrolment(ctx); err == nil {
		t.Fatal("re-enrolment over a confirmed factor was allowed")
	}

	// Disabling a confirmed factor is a step-up: a session alone (here, a wrong
	// code) must not strip it, or a hijacked session enrols its own factor next.
	if err := e.svc.DisableMFA(ctx, "000000"); !errors.Is(err, errBadMFACode) {
		t.Fatalf("disable with a wrong code: err=%v, want errBadMFACode", err)
	}
	if !statusOf(t, e).Enrolled {
		t.Fatal("a refused disable still removed the factor")
	}

	// An unused recovery code is a valid step-up — it is the disable a member
	// reaches for when the authenticator is exactly what they lost.
	if err := e.svc.DisableMFA(ctx, recovery[0]); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if statusOf(t, e).Enrolled {
		t.Error("MFA still reads as enrolled after disable")
	}
	// The codes hang off the enrolment row, so none may survive it: a stale
	// live code would re-enter the account after the factor was removed.
	var stale int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM mfa_recovery_code WHERE user_id = $1`, e.member.UserID).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Errorf("%d recovery codes survived the disable", stale)
	}

	// Disabling again with nothing enrolled stays a no-op, whatever the code.
	if err := e.svc.DisableMFA(ctx, "000000"); err != nil {
		t.Fatalf("idempotent disable: %v", err)
	}

	// After disabling, enrolment can start again, and confirming issues a full
	// fresh set — no residue from the previous factor.
	fresh, err := e.svc.StartTOTPEnrolment(ctx)
	if err != nil {
		t.Fatalf("re-enrolment after disable was refused: %v", err)
	}
	freshCode, _ := totpCodeAt(fresh, base, totpDigits)
	if _, err := e.svc.ConfirmTOTP(ctx, freshCode, ""); err != nil {
		t.Fatalf("confirm after re-enrol: %v", err)
	}
	if left := statusOf(t, e).RecoveryCodesLeft; left != recoveryCodeCount {
		t.Errorf("recovery codes after re-enrol = %d, want a full fresh set of %d", left, recoveryCodeCount)
	}
}

func TestConfirmTOTPRefusesAWrongCode(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-wrongcode")
	e.withVault()
	ctx := e.asMember()

	if _, err := e.svc.StartTOTPEnrolment(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := e.svc.ConfirmTOTP(ctx, "000000", ""); err == nil {
		t.Fatal("a wrong confirmation code was accepted")
	}
	// The enrolment stays pending, not confirmed.
	if statusOf(t, e).Confirmed {
		t.Error("a wrong code still confirmed the enrolment")
	}
}

func TestConfirmTOTPEndsEveryOtherSessionOfTheMember(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-cutover")
	e.withVault()

	// Two devices signed in before any factor existed.
	_, keep, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice)
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	_, other, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword, noDevice)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	ctx := e.asMember()
	secret, err := e.svc.StartTOTPEnrolment(ctx)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, err := e.svc.ConfirmTOTP(ctx, code, hashToken(keep.Token)); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// The confirming session survives; every session that predates the factor
	// is dead — it was opened when a password alone sufficed, and confirming
	// must not launder it into a factor-backed one.
	if _, err := e.svc.Authenticate(e.wsOnlyCtx(), keep.Token); err != nil {
		t.Fatalf("the confirming session did not survive enrolment: %v", err)
	}
	if _, err := e.svc.Authenticate(e.wsOnlyCtx(), other.Token); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the pre-factor session still authenticates: err=%v, want ErrNotFound", err)
	}
}

func statusOf(t *testing.T, e *revocationEnv) MFAStatus {
	t.Helper()
	status, err := e.svc.MFAEnrolment(e.asMember())
	if err != nil {
		t.Fatalf("MFAEnrolment: %v", err)
	}
	return status
}
