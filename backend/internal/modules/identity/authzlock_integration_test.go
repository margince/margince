// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Every write that decides or changes who may do what waits for the one
// authorization lock, and checks containment only once it holds it. A role
// widened while a check waits is the role the check then reads.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// holdAuthorization takes the authorization lock in a transaction of its own
// and returns it; the lock is held until the caller commits or rolls back.
func (e *revocationEnv) holdAuthorization(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := lockAuthorization(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	return tx
}

// waitForAnAuthorizationWaiter returns once a backend is blocked on the
// authorization lock.
func waitForAnAuthorizationWaiter(t *testing.T, tx pgx.Tx) {
	t.Helper()
	for range 10_000 {
		var waiting bool
		if err := tx.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
	}
	t.Fatal("the write never waited on the authorization lock")
}

func TestEveryAuthorizationChangeWaitsForTheLock(t *testing.T) {
	e := setupRevocationEnv(t, "authz-lock")
	ctx := e.wsCtx(e.admin)
	custom := e.customRole(t, "Seasonal", "rep", nil)
	archived := e.customRole(t, "Retired", "rep", nil)
	if _, err := e.svc.ArchiveRole(ctx, e.admin, archived); err != nil {
		t.Fatal(err)
	}
	team, err := e.svc.CreateTeam(ctx, e.admin, "North")
	if err != nil {
		t.Fatal(err)
	}
	member := e.seat(t, "colleague", "rep")
	gone := e.seat(t, "gone", "rep")
	if err := e.svc.DeactivateUser(ctx, e.admin, DeactivateUserInput{UserID: gone.UserID}); err != nil {
		t.Fatal(err)
	}
	name, scope := "Seasonal EMEA", principal.RowScopeTeam

	// In order: the last row archives the role the first row made.
	for _, step := range []struct {
		verb string
		call func() error
	}{
		{"create a role", func() error { _, err := e.svc.CreateRole(ctx, e.admin, "rep", "Copy"); return err }},
		{"update a role", func() error {
			_, err := e.svc.UpdateRole(ctx, e.admin, custom, RoleChange{Name: &name, RowScope: &scope}, nil)
			return err
		}},
		{"restore a role", func() error { _, err := e.svc.RestoreRole(ctx, e.admin, archived); return err }},
		{"edit a grant", func() error {
			_, err := e.svc.SetRoleObjectGrant(ctx, e.admin, custom, "deal", storedGrant{Read: true}, nil)
			return err
		}},
		{"invite", func() error {
			_, _, err := e.svc.InviteUser(ctx, e.admin, InviteUserInput{Email: "new@" + e.slug + ".test", DisplayName: "New", Role: "rep"})
			return err
		}},
		{"record a former member", func() error {
			_, err := e.svc.CreateFormerMember(ctx, e.admin, FormerMemberInput{Email: "old@" + e.slug + ".test", DisplayName: "Old"})
			return err
		}},
		{"change a role", func() error { return e.svc.ChangeUserRole(ctx, e.admin, member.UserID, custom) }},
		{"join a team", func() error { return e.svc.SetTeamMember(ctx, e.admin, team.ID, member.UserID.UUID, true) }},
		{"rename a team", func() error {
			_, err := e.svc.UpdateTeam(ctx, e.admin, team.ID, UpdateTeamInput{Name: &name})
			return err
		}},
		{"reactivate", func() error { return e.svc.ReactivateUser(ctx, e.admin, gone.UserID) }},
		{"issue a link", func() error { _, _, err := e.svc.IssuePasswordLink(ctx, e.admin, member.UserID); return err }},
		{"issue a passport", func() error {
			_, err := e.svc.IssuePassport(ctx, e.admin, IssuePassportInput{Scopes: []string{"read"}})
			return err
		}},
		{"deactivate", func() error { return e.svc.DeactivateUser(ctx, e.admin, DeactivateUserInput{UserID: member.UserID}) }},
		{"archive a role", func() error { _, err := e.svc.ArchiveRole(ctx, e.admin, "custom_copy"); return err }},
	} {
		held := e.holdAuthorization(t)
		done := make(chan error, 1)
		go func() { done <- step.call() }()
		waitForAnAuthorizationWaiter(t, held)
		if err := held.Rollback(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Errorf("%s, once the lock was free: %v", step.verb, err)
		}
	}
}

// A role widened while a password link waits for the lock is the role the
// link's check reads, so the link is refused rather than issued against the
// narrower role that stood when the request arrived.
func TestALinkWaitingOnTheLockSeesTheRoleWidenedMeanwhile(t *testing.T) {
	e := setupRevocationEnv(t, "authz-toctou")
	delegate := e.seat(t, "delegate", e.customRole(t, "Member admin", "rep", map[string]storedGrant{objectUserAdmin: fullGrant}))
	targetRole := e.customRole(t, "Plain rep", "rep", nil)
	target := e.seat(t, "target", targetRole)

	held := e.holdAuthorization(t)
	done := make(chan error, 1)
	go func() {
		_, _, err := e.svc.IssuePasswordLink(e.wsCtx(delegate), delegate, target.UserID)
		done <- err
	}()
	waitForAnAuthorizationWaiter(t, held)
	if _, err := held.Exec(context.Background(),
		`UPDATE role SET permissions = jsonb_set(permissions, '{objects,audit_log}',
		        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
		  WHERE key = $1`, targetRole); err != nil {
		t.Fatal(err)
	}
	if err := held.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a link for a target widened while it waited: %v, want permission denied", err)
	}
	var issued int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM auth_token WHERE user_id = $1 AND purpose = 'password_reset' AND used_at IS NULL`,
		target.UserID).Scan(&issued); err != nil || issued != 1 {
		t.Errorf("the target holds %d live set-password tokens (%v), want only the invitation's", issued, err)
	}
}
