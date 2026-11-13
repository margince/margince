// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const testAppIssuer = "https://chat.example.test"

// appCtx is what the edge hands identity: the workspace and nobody acting yet.
func (e *revocationEnv) appCtx() context.Context {
	return principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws.UUID), ids.NewV7())
}

func (e *revocationEnv) appCaller(subject, email string) AppCaller {
	return AppCaller{Issuer: testAppIssuer, Subject: subject, Email: email, Label: "Dispact"}
}

func (e *revocationEnv) appPassports(t *testing.T, subject string) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM passport WHERE app_issuer = $1 AND app_subject = $2`, testAppIssuer, subject).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAnAppCallerGetsOnePassportForTheMemberItNames(t *testing.T) {
	e := setupRevocationEnv(t, "app-caller-mint")
	first, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.member.Email))
	if err != nil {
		t.Fatalf("the first call: %v", err)
	}
	if first.OnBehalfOf != e.member.UserID {
		t.Fatalf("the passport acts for %v, want the member %v", first.OnBehalfOf, e.member.UserID)
	}
	if !first.Scopes.Has(principal.ScopeRead) || !first.Scopes.Has(principal.ScopeSend) {
		t.Fatalf("the app passport carries %v", first.Scopes)
	}
	again, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.member.Email))
	if err != nil {
		t.Fatalf("the second call: %v", err)
	}
	if again.PassportID != first.PassportID || e.appPassports(t, "u-1") != 1 {
		t.Fatalf("the second call used passport %v, want the first one %v, and one row", again.PassportID, first.PassportID)
	}
	var audited int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'passport' AND entity_id = $1 AND action = 'create'`,
		first.PassportID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Fatalf("minting the app passport left %d audit rows, want 1", audited)
	}
}

func TestARevokedAppPassportStaysRefused(t *testing.T) {
	e := setupRevocationEnv(t, "app-caller-revoked")
	agent, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.member.Email))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RevokePassport(e.wsCtx(e.member), e.member, agent.PassportID); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if _, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.member.Email)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a call after the revoke answered %v, want not found", err)
	}
	if n := e.appPassports(t, "u-1"); n != 1 {
		t.Fatalf("the revoke was answered by minting a new passport (%d rows)", n)
	}
}

func TestAChangedEmailIsRefused(t *testing.T) {
	e := setupRevocationEnv(t, "app-caller-email")
	if _, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.member.Email)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.admin.Email)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the same chat user vouched for another member's address and got %v, want not found", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE app_user SET email = 'moved-'||email WHERE id = $1`, e.member.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.member.Email)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a member whose login changed still matched the old address: %v", err)
	}
}

func TestAnAddressNoMemberHoldsHasNoSeat(t *testing.T) {
	e := setupRevocationEnv(t, "app-caller-noseat")
	if _, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-9", "nobody@elsewhere.test")); !errors.Is(err, ErrNoSeat) {
		t.Fatalf("an unknown address answered %v, want ErrNoSeat", err)
	}
	if n := e.appPassports(t, "u-9"); n != 0 {
		t.Fatalf("an unknown address left %d passports", n)
	}
}

func TestAnExpiredAppPassportIsRenewedForTheSameMemberOnly(t *testing.T) {
	e := setupRevocationEnv(t, "app-caller-expired")
	first, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.member.Email))
	if err != nil {
		t.Fatal(err)
	}
	expire := func() {
		if _, err := e.owner.Exec(context.Background(),
			`UPDATE passport SET created_at = now() - interval '2 days', expires_at = now() - interval '1 day'
			  WHERE app_subject = 'u-1'`); err != nil {
			t.Fatal(err)
		}
	}
	expire()
	renewed, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.member.Email))
	if err != nil {
		t.Fatalf("renewing: %v", err)
	}
	if renewed.PassportID == first.PassportID || renewed.OnBehalfOf != e.member.UserID {
		t.Fatalf("the renewal is %+v", renewed)
	}
	expire()
	if _, err := e.svc.AuthenticateAppCaller(e.appCtx(), e.appCaller("u-1", e.admin.Email)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an expired passport was renewed for another member: %v", err)
	}
}
