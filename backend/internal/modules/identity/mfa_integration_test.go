// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The MFA store end to end over a real database and a real (in-memory) seal:
// enrol an authenticator, confirm it into an active factor with recovery codes,
// verify both a live code and a one-time recovery code, and tear it down. The
// secret never lands in the row — only its vault ref does.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *revocationEnv) withVault() {
	e.svc.WithVault(keyvault.NewMemory())
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

	code, err := totpCodeAt(secret, e.svc.now(), totpDigits)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := e.svc.ConfirmTOTP(ctx, code)
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

	// A live authenticator code verifies.
	fresh, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if !e.verifyMemberMFA(t, fresh) {
		t.Error("a valid authenticator code did not verify")
	}

	// A recovery code verifies once and is then spent.
	if !e.verifyMemberMFA(t, recovery[0]) {
		t.Error("a recovery code did not verify")
	}
	if e.verifyMemberMFA(t, recovery[0]) {
		t.Error("a recovery code verified a second time; it must be single-use")
	}
	if left := statusOf(t, e, ctx).RecoveryCodesLeft; left != recoveryCodeCount-1 {
		t.Errorf("recovery codes left = %d, want %d after spending one", left, recoveryCodeCount-1)
	}
}

func TestMFACannotBeReEnrolledWhileConfirmed(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-reenrol")
	e.withVault()
	ctx := e.asMember()

	secret, err := e.svc.StartTOTPEnrolment(ctx)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, err := e.svc.ConfirmTOTP(ctx, code); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// A confirmed factor must be disabled before a new one can be started, or a
	// silent re-enrolment would swap the factor under the member.
	if _, err := e.svc.StartTOTPEnrolment(ctx); err == nil {
		t.Fatal("re-enrolment over a confirmed factor was allowed")
	}

	if err := e.svc.DisableMFA(ctx); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if statusOf(t, e, ctx).Enrolled {
		t.Error("MFA still reads as enrolled after disable")
	}
	// After disabling, enrolment can start again.
	if _, err := e.svc.StartTOTPEnrolment(ctx); err != nil {
		t.Fatalf("re-enrolment after disable was refused: %v", err)
	}
}

func TestConfirmTOTPRefusesAWrongCode(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-wrongcode")
	e.withVault()
	ctx := e.asMember()

	if _, err := e.svc.StartTOTPEnrolment(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := e.svc.ConfirmTOTP(ctx, "000000"); err == nil {
		t.Fatal("a wrong confirmation code was accepted")
	}
	// The enrolment stays pending, not confirmed.
	if statusOf(t, e, ctx).Confirmed {
		t.Error("a wrong code still confirmed the enrolment")
	}
}

func statusOf(t *testing.T, e *revocationEnv, ctx context.Context) MFAStatus {
	t.Helper()
	status, err := e.svc.MFAEnrolment(ctx)
	if err != nil {
		t.Fatalf("MFAEnrolment: %v", err)
	}
	return status
}
