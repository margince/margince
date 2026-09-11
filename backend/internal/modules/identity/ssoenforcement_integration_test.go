// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Enforced-SSO mode: when an installation requires single sign-on, the password
// path is closed to ordinary members but stays open to admins — the break-glass
// that keeps a broken IdP from locking out the people who fix it. A wrong
// password is still refused first and neutrally, so enforcement never becomes a
// password oracle.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func enforceSSO(e *revocationEnv) {
	e.svc.requireSSO = func(context.Context) (bool, error) { return true, nil }
}

func sessionCount(t *testing.T, e *revocationEnv, userID ids.UserID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM session WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return n
}

func TestEnforcedSSORefusesANonAdminPasswordLogin(t *testing.T) {
	e := setupRevocationEnv(t, "sso-enforced")
	enforceSSO(e)

	_, token, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword)
	if !errors.Is(err, errSSORequired) {
		t.Fatalf("non-admin under enforced SSO: err=%v, want errSSORequired", err)
	}
	if token != "" {
		t.Error("a refused SSO-required login must mint no session token")
	}
	if n := sessionCount(t, e, e.member.UserID); n != 0 {
		t.Errorf("a refused login left %d session rows, want 0", n)
	}
}

func TestEnforcedSSOKeepsAdminPasswordLoginAsBreakGlass(t *testing.T) {
	e := setupRevocationEnv(t, "sso-breakglass")
	enforceSSO(e)

	id, token, err := e.svc.Login(e.wsOnlyCtx(), e.admin.Email, bootstrapPassword)
	if err != nil {
		t.Fatalf("admin break-glass login refused under enforced SSO: %v", err)
	}
	if token == "" || !id.hasRole(roleAdmin) {
		t.Fatalf("break-glass admin login did not land as an admin session: token=%q roles=%v", token, id.Roles)
	}
}

func TestEnforcedSSOStillRefusesAWrongPasswordNeutrally(t *testing.T) {
	e := setupRevocationEnv(t, "sso-wrongpw")
	enforceSSO(e)

	// A wrong password is refused by the credential check FIRST, so enforcement
	// never reveals that a password was right by answering differently.
	_, _, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, "not the password")
	if !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("wrong password under enforced SSO: err=%v, want ErrBadCredentials (not sso_required)", err)
	}
}

func TestSSONotEnforcedLetsAMemberLogInWithPassword(t *testing.T) {
	e := setupRevocationEnv(t, "sso-off")
	// requireSSO left nil: an installation that never set the policy behaves
	// exactly as before.
	_, token, err := e.svc.Login(e.wsOnlyCtx(), e.member.Email, memberPassword)
	if err != nil || token == "" {
		t.Fatalf("member password login with SSO not enforced: err=%v token=%q", err, token)
	}
}
