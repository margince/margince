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
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	waitForABlockedBackend(t, tx, `locktype = 'advisory'`)
}

// waitForABlockedBackend returns once a backend is blocked on a lock of the kind
// the pg_locks predicate names.
func waitForABlockedBackend(t *testing.T, tx pgx.Tx, kind string) {
	t.Helper()
	for range 10_000 {
		var waiting bool
		if err := tx.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE `+kind+` AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
	}
	t.Fatalf("no write ever waited on a lock where %s", kind)
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

// agentOf is ctx for an agent acting on a passport lent by user.
func (e *revocationEnv) agentOf(user Identity) context.Context {
	return principal.WithActor(e.wsCtx(user), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(), UserID: user.UserID.UUID,
		PassportID: ids.NewV7(), OnBehalfOf: user.UserID.UUID, SeatType: principal.SeatType(user.SeatType),
		Permissions: user.Permissions,
	})
}

// A colleague promoted to administrator while an agent's team change is running
// is an administrator to that change, so the agent limit refuses it. The check
// sits after the lock the promotion holds, and so reads the promotion.
func TestAnAgentWaitingOnTheLockSeesAColleaguePromotedMeanwhile(t *testing.T) {
	e := setupRevocationEnv(t, "agent-toctou")
	ctx := e.wsCtx(e.admin)
	team, err := e.svc.CreateTeam(ctx, e.admin, "Waiting")
	if err != nil {
		t.Fatal(err)
	}
	target := e.seat(t, "target", "rep")

	held := e.holdAuthorization(t)
	if _, err := held.Exec(context.Background(),
		`UPDATE role_assignment SET role_id = (SELECT id FROM role WHERE key = 'admin') WHERE user_id = $1`,
		target.UserID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- e.svc.SetTeamMember(e.agentOf(e.admin), e.admin, team.ID, target.UserID.UUID, true)
	}()
	waitForAnAuthorizationWaiter(t, held)
	if err := held.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	var limit *httperr.DetailedError
	if err := <-done; !errors.As(err, &limit) || limit.Code != "agent_member_limit" {
		t.Errorf("an agent's change to a colleague promoted meanwhile: %v, want the agent limit", err)
	}
}

// A handover admits on the member's locked row, so a promotion holding that row
// is read once it commits. A check before the lock would read the old role.
func TestAnAgentHandoverWaitingOnTheMemberSeesThePromotion(t *testing.T) {
	e := setupRevocationEnv(t, "agent-handover-toctou")
	target := e.seat(t, "target", "rep")

	held, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := held.Exec(context.Background(), `SELECT 1 FROM app_user WHERE id = $1 FOR UPDATE`, target.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := held.Exec(context.Background(),
		`UPDATE role_assignment SET role_id = (SELECT id FROM role WHERE key = 'admin') WHERE user_id = $1`,
		target.UserID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		agent := e.agentOf(e.admin)
		done <- e.svc.db.Tx(agent, func(tx pgx.Tx) error { return AdmitMemberHandover(agent, tx, target.UserID) })
	}()
	waitForABlockedBackend(t, held, `locktype <> 'advisory'`)
	if err := held.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	var limit *httperr.DetailedError
	if err := <-done; !errors.As(err, &limit) || limit.Code != "agent_member_limit" {
		t.Errorf("an agent's handover from a colleague promoted meanwhile: %v, want the agent limit", err)
	}
}

// A delegated act that takes no lock of its own still waits for a promotion. So
// an admin-backed agent cannot act on a colleague promoted meanwhile.
func TestAnAgentActingWithoutTheLockStillSeesAPromotion(t *testing.T) {
	e := setupRevocationEnv(t, "agent-session-toctou")
	target := e.seat(t, "target", "rep")
	held := e.holdAuthorization(t)
	if _, err := held.Exec(context.Background(),
		`UPDATE role_assignment SET role_id = (SELECT id FROM role WHERE key = 'admin') WHERE user_id = $1`,
		target.UserID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- e.svc.RevokeUserSession(e.agentOf(e.admin), e.admin, target.UserID, ids.NewV7())
	}()
	waitForAnAuthorizationWaiter(t, held)
	if err := held.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	var limit *httperr.DetailedError
	if err := <-done; !errors.As(err, &limit) || limit.Code != "agent_member_limit" {
		t.Errorf("an agent's act on a colleague promoted meanwhile: %v, want the agent limit", err)
	}
}

func TestAnAgentEditingAMemberNobodyHoldsAnswersNotFound(t *testing.T) {
	e := setupRevocationEnv(t, "agent-edit-unknown")
	err := e.svc.UpdateMember(e.agentOf(e.admin), e.admin, ids.From[ids.UserKind](ids.NewV7()), MemberEdit{Name: "Nobody", Email: "nobody@example.test"})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an agent editing an unknown member: %v, want not found", err)
	}
}

// A preview is what a confirmation is staged from, so the agent limits hold here
// as they hold when the removal runs.
func TestAnAgentCannotPreviewARemovalItCouldNotRun(t *testing.T) {
	e := setupRevocationEnv(t, "agent-retire-preview")
	agent := e.agentOf(e.admin)
	admin := e.seat(t, "second-admin", "admin")
	colleague := e.seat(t, "colleague", "rep")
	own := e.admin.UserID.UUID
	for name, call := range map[string]func() error{
		"an administrator": func() error {
			_, err := e.svc.PreviewMemberRetirement(agent, e.admin, admin.UserID, nil, false)
			return err
		},
		"work given to its own user": func() error {
			_, err := e.svc.PreviewMemberRetirement(agent, e.admin, colleague.UserID, &own, false)
			return err
		},
	} {
		var limit *httperr.DetailedError
		if err := call(); !errors.As(err, &limit) || limit.Code != "agent_member_limit" {
			t.Errorf("%s: %v, want the agent limit", name, err)
		}
	}
}

// countingTx counts the statements a check sends.
type countingTx struct {
	pgx.Tx
	queries int
}

func (c *countingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.queries++
	return c.Tx.Query(ctx, sql, args...)
}

func (c *countingTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.queries++
	return c.Tx.QueryRow(ctx, sql, args...)
}

// The agent's team check reads a whole team at once, so a large team does not
// hold the transaction for a round trip per member.
func TestAnAgentsTeamCheckDoesNotGrowWithTheTeam(t *testing.T) {
	e := setupRevocationEnv(t, "agent-team-batch")
	ctx := e.wsCtx(e.admin)
	team, err := e.svc.CreateTeam(ctx, e.admin, "Large")
	if err != nil {
		t.Fatal(err)
	}
	for _, handle := range []string{"one", "two", "three", "four"} {
		member := e.seat(t, handle, "rep")
		if err := e.svc.SetTeamMember(ctx, e.admin, team.ID, member.UserID.UUID, true); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	counted := &countingTx{Tx: tx}
	if err := refuseAgentTeamReach(e.agentOf(e.admin), counted, team.ID); err != nil {
		t.Fatal(err)
	}
	if counted.queries != 2 {
		t.Errorf("the check sent %d queries for four members, want 2: the members, then their roles", counted.queries)
	}
	admin := e.seat(t, "team-admin", "admin")
	if err := e.svc.SetTeamMember(ctx, e.admin, team.ID, admin.UserID.UUID, true); err != nil {
		t.Fatal(err)
	}
	var limit *httperr.DetailedError
	if err := refuseAgentTeamReach(e.agentOf(e.admin), tx, team.ID); !errors.As(err, &limit) {
		t.Errorf("a team holding an administrator: %v, want the agent limit", err)
	}
}

// dealUpdateInTx reads the member's authority the way a deal write does and
// reports whether it grants deal:update.
func (e *revocationEnv) dealUpdateInTx(ctx context.Context, tx pgx.Tx, member Identity) (bool, error) {
	rbac, _, err := e.svc.EffectiveAuthorityTx(ctx, tx, member.UserID.UUID)
	return rbac.Permissions.Allows("deal", principal.ActionUpdate), err
}

// dealCloser puts the env's live member on a custom role that grants deal:update.
func (e *revocationEnv) dealCloser(t *testing.T) (string, Identity) {
	t.Helper()
	role := e.customRole(t, "Closer", "rep", map[string]storedGrant{"deal": fullGrant})
	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, role); err != nil {
		t.Fatal(err)
	}
	return role, e.member
}

// A role edit that starts after a write read its owner's authority waits until
// that write commits. So the write never lands on a grant already taken away.
func TestARoleEditWaitsForTheWriteAnAuthorityReadAdmitted(t *testing.T) {
	e := setupRevocationEnv(t, "authority-hold")
	ctx := e.wsCtx(e.admin)
	role, owner := e.dealCloser(t)

	edited := make(chan error, 1)
	err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		allowed, err := e.dealUpdateInTx(ctx, tx, owner)
		if err != nil || !allowed {
			t.Fatalf("the owner's authority before the edit: deal:update=%v, %v", allowed, err)
		}
		go func() {
			_, err := e.svc.SetRoleObjectGrant(ctx, e.admin, role, "deal", storedGrant{Read: true}, nil)
			edited <- err
		}()
		waitForAnAuthorizationWaiter(t, tx)
		select {
		case err := <-edited:
			t.Fatalf("the edit returned (%v) while the write it would narrow was still open", err)
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-edited; err != nil {
		t.Fatalf("the edit, once the write committed: %v", err)
	}
	err = e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		allowed, err := e.dealUpdateInTx(ctx, tx, owner)
		if allowed {
			t.Error("the owner still holds deal:update after the edit")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// An authority read that starts while a role edit is open waits for the edit,
// and reads the narrowed grant.
func TestAnAuthorityReadWaitsForARoleEditInProgress(t *testing.T) {
	e := setupRevocationEnv(t, "authority-wait")
	ctx := e.wsCtx(e.admin)
	role, owner := e.dealCloser(t)

	applied, release, edited := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	finishEdit := sync.OnceFunc(func() { close(release) })
	t.Cleanup(finishEdit)
	go func() {
		edited <- e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
			if err := lockAuthorization(ctx, tx); err != nil {
				return err
			}
			if _, err := applyRoleObjectGrant(ctx, tx, e.admin, role, "deal", storedGrant{Read: true}, nil); err != nil {
				return err
			}
			close(applied)
			<-release
			return nil
		})
	}()
	<-applied
	read := make(chan bool, 1)
	go func() {
		err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
			allowed, err := e.dealUpdateInTx(ctx, tx, owner)
			read <- allowed
			return err
		})
		if err != nil {
			t.Error(err)
		}
	}()
	probe, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitForAnAuthorizationWaiter(t, probe)
	if err := probe.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	finishEdit()
	if err := <-edited; err != nil {
		t.Fatal(err)
	}
	if <-read {
		t.Error("an authority read waiting on the edit still granted deal:update")
	}
}

// An authority hold keeps the member's row from changing while its write is
// open. A write that only names the member in a foreign key goes on beside it.
// So a write that locked a record first cannot wait on the hold.
func TestAnAuthorityHoldLetsAWriteNameTheMember(t *testing.T) {
	e := setupRevocationEnv(t, "authority-reference")
	ctx := e.wsCtx(e.admin)
	_, member := e.dealCloser(t)
	err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := e.dealUpdateInTx(ctx, tx, member); err != nil {
			return err
		}
		// The check every foreign key to app_user makes, and a change to the row.
		e.wantMemberLock(t, member, "FOR KEY SHARE", false)
		e.wantMemberLock(t, member, "FOR NO KEY UPDATE", true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// wantMemberLock takes mode on the member's row without waiting, in a
// transaction of its own. It fails unless the hold refuses it when refused says so.
func (e *revocationEnv) wantMemberLock(t *testing.T, member Identity, mode string, refused bool) {
	t.Helper()
	probe, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := probe.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	_, err = probe.Exec(context.Background(),
		`SELECT 1 FROM app_user WHERE id = $1 `+mode+` NOWAIT`, member.UserID.UUID)
	switch {
	case err == nil && refused:
		t.Errorf("%s on the member went through beside the authority hold", mode)
	case err != nil && !storekit.IsLockTimeout(err):
		t.Fatalf("%s on the member: %v", mode, err)
	case err != nil && !refused:
		t.Errorf("%s on the member waits for the authority hold", mode)
	}
}
