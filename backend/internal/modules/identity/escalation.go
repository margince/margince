// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Who may act on WHOM, once administering members is a grant rather than a role.
//
// While `user_admin`'s verbs were reachable only by an admin, no call site had
// to read the target's role: every caller already held every authority every
// target could hold, so there was nothing to escalate to. Handing the grant to a
// non-admin removes that guarantee, and custom roles remove the guarantee that
// only an admin can be wider than the caller. Three paths end at the same place
// — an account wider than the caller's under the caller's control:
//
//   - IssuePasswordLink mints a single-use password token for any user and
//     returns the raw value. A delegated holder could take over a wider
//     account outright, which is why the seat check beside it already says a
//     caller must not mint one "on the strength of a role alone".
//   - DeactivateUser and ReactivateUser read only status and seat. A delegated
//     holder could lock out every admin but the last, or restore one somebody
//     removed.
//   - ChangeUserRole and InviteUser could hand out a role wider than the
//     caller's own — the role editor itself, or the installation reset.
//
// The rule is one sentence: a caller may only act on a target whose authority
// their own contains. The literal admin role is its own ceiling, so only a
// literal admin acts on one; no grant substitutes, because a role that could
// grant itself admin-equivalence would make the ceiling meaningless. How much
// of the target must be contained depends on the verb (targetReach).
//
// Read inside the caller's transaction, never before it. A role assignment
// changed between the check and the write would otherwise decide the answer
// against a state that no longer holds.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The RBAC objects this module's settings surfaces are gated on.
//
// Constants rather than string literals at the call sites: a misspelt literal
// resolves to the zero grant and refuses everybody, which reads in production as
// a broken feature rather than as a typo, and no compiler notices.
//
// Held by: TestIdentityNamesItsRbacObjectsThroughConstants
// (backend/internal/modules/identity/escalation_test.go), which fails on a bare
// literal of any of these names anywhere else in the package.
const (
	objectUserAdmin = "user_admin"
	objectRoleAdmin = "role_admin"
	objectTeamAdmin = "team_admin"
	objectTeamLead  = "team_lead"
)

// admit puts the caller on the context and asks whether the grant admits them,
// in that order and as one call.
//
// The order is the whole reason it is a function. auth.Require reads the actor
// off the context, so a site that checks before calling actorCtx asks about
// nobody and refuses everybody — a mistake that reads in production as a broken
// feature rather than as a missing line, and one that seventeen hand-written
// pairs would eventually make.
func admit(ctx context.Context, actor Identity, object string, action principal.Action) (context.Context, error) {
	ctx = actorCtx(ctx, actor)
	return ctx, auth.Require(ctx, object, action)
}

// lockTargetGrants resolves what a user holds, the way login resolves it,
// locking the USER row first so the set cannot change under the mutation that
// follows.
//
// The lock is on app_user and not on role_assignment, and the difference is load
// bearing. A row lock at READ COMMITTED locks rows that EXIST; there is no gap
// lock, so `FOR UPDATE` over the assignments of a target who currently holds
// none locks nothing at all. ChangeUserRole re-roles by DELETE-then-INSERT, so
// every target passes through exactly that state mid-transaction — and a guard
// reading it there would see no roles, admit the caller, and let the write land
// against a user who commits as an admin a moment later.
//
// A user with no assignment is not an error: an agent seat holds no role, and
// neither does a member whose last role was removed. Both are targets a caller
// may act on, because an empty authority is contained by every authority.
func lockTargetGrants(ctx context.Context, tx pgx.Tx, userID ids.UserID) (seatGrants, error) {
	var exists bool
	switch err := tx.QueryRow(ctx,
		`SELECT true FROM app_user WHERE id = $1 AND archived_at IS NULL FOR UPDATE`,
		userID).Scan(&exists); {
	case errors.Is(err, pgx.ErrNoRows):
		return seatGrants{}, apperrors.ErrNotFound
	case err != nil:
		return seatGrants{}, err
	}
	all, err := loadGrantsFor(ctx, tx, []ids.UUID{userID.UUID})
	if err != nil {
		return seatGrants{}, err
	}
	return all[userID.UUID], nil
}

// targetReach is how much of the target a verb puts in the caller's hands.
type targetReach int

const (
	// reachDenial takes access away or gives it back — deactivate, reactivate,
	// revoke a passport. The caller must hold the target's administration
	// grants and row scope, because those are what locking somebody out, or
	// letting them back in, has power over.
	reachDenial targetReach = iota
	// reachTakeover puts the account itself in the caller's hands or reshapes
	// all of it: a password link hands out a credential, and a role change
	// replaces everything the target may do. The caller must hold everything
	// the target holds.
	reachTakeover
)

// refuseUnlessCallerOutranksTarget is the guard every delegated member verb
// carries. It answers one question: may this caller act on this target at all.
//
// The literal admin role is checked first and is its own ceiling: a non-admin
// never acts on an admin, whatever grants they hold. Every other target is
// decided by containment, over the span the verb reaches.
//
// Returns ErrPermissionDenied and not ErrNotFound: the target's existence is
// already disclosed by the roster this caller may read, so hiding it here would
// buy nothing and would tell an operator their own action failed for the wrong
// reason.
func refuseUnlessCallerOutranksTarget(ctx context.Context, tx pgx.Tx, actor Identity, targetID ids.UserID, reach targetReach) error {
	if actor.hasRole(roleAdmin) {
		return nil
	}
	target, err := lockTargetGrants(ctx, tx, targetID)
	if err != nil {
		return err
	}
	return callerOutranks(actor, target, reach)
}

// callerOutranks decides the ceiling over grants already read. The roster's
// allowed actions ask it too, so the offer and the write share one answer.
func callerOutranks(actor Identity, target seatGrants, reach targetReach) error {
	if actor.hasRole(roleAdmin) {
		return nil
	}
	if slices.Contains(target.roles, roleAdmin) {
		return apperrors.ErrPermissionDenied
	}
	if !containsAdministration(actor.Permissions, target.perms) {
		return apperrors.ErrPermissionDenied
	}
	if reach == reachTakeover && !containsWholeAccess(actor, target) {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// containsAdministration reports whether the caller holds every administration
// grant the other side confers, and a row scope at least as wide.
func containsAdministration(caller, other principal.Permissions) bool {
	for _, object := range administrationObjects {
		if !caller.Objects[object].Contains(other.Objects[object]) {
			return false
		}
	}
	return !other.RowScope.Wider(caller.RowScope)
}

// containsWholeAccess reports whether the caller may already do everything the
// target may: every object grant, the row scope, the teams that scope and the
// lead grant reach, and every field the target can read.
func containsWholeAccess(actor Identity, target seatGrants) bool {
	for object, grant := range target.perms.Objects {
		if !actor.Permissions.Objects[object].Contains(grant) {
			return false
		}
	}
	if target.perms.RowScope.Wider(actor.Permissions.RowScope) {
		return false
	}
	return teamReachContained(actor, target) &&
		masksContained(actor.Permissions.FieldMasks, target.perms.FieldMasks)
}

// teamReachContained asks about the teams the target is on and the caller is
// not. They matter only where the target's authority follows its teams: a
// team row scope reads those teams' records, which only an unbounded caller
// already reads, and the team_lead grant coaches and reads those teams' weeks,
// which no caller outside the team does.
func teamReachContained(actor Identity, target seatGrants) bool {
	outside := slices.ContainsFunc(target.teams, func(team ids.TeamID) bool {
		return !slices.Contains(actor.Teams, team)
	})
	if !outside {
		return true
	}
	if target.perms.RowScope == principal.RowScopeTeam && actor.Permissions.RowScope != principal.RowScopeAll {
		return false
	}
	return target.perms.Objects[objectTeamLead] == principal.ObjectGrant{}
}

// masksContained reports whether every field the caller reads as withheld is
// withheld from the target too, at least as often. A mask the caller carries
// and the target lacks is a field the target reads and the caller cannot.
func masksContained(caller, target []principal.FieldMask) bool {
	for _, mask := range caller {
		covered := slices.ContainsFunc(target, func(held principal.FieldMask) bool {
			return held.Object == mask.Object && held.Field == mask.Field &&
				(held.Condition == principal.MaskAlways || held.Condition == mask.Condition)
		})
		if !covered {
			return false
		}
	}
	return true
}

// roleForAssignment reads the live role a key names, refuses it when this
// caller may not hand it to an account on these teams, and returns its id for
// the assignment row.
//
// Its caller holds lockAuthorization, which archiving takes too, so the role
// cannot be archived between this read and the assignment. An archived role
// answers errUnknownRole — it is not a role anybody can be given.
func roleForAssignment(ctx context.Context, tx pgx.Tx, actor Identity, roleKey string, teams []ids.TeamID) (ids.UUID, error) {
	var roleID ids.UUID
	var raw []byte
	err := tx.QueryRow(ctx,
		`SELECT id, permissions FROM role WHERE key = $1 AND archived_at IS NULL`,
		roleKey).Scan(&roleID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, errUnknownRole
	}
	if err != nil {
		return ids.UUID{}, err
	}
	if err := refuseUnlessCallerMayAssign(ctx, tx, actor, roleKey, raw, teams); err != nil {
		return ids.UUID{}, err
	}
	return roleID, nil
}

// refuseUnlessCallerMayAssign answers the other half: may this caller produce
// an account holding THIS role on these teams. Handing out a role is handing
// out an account — an invite mails its link to an address the caller chooses —
// so a caller who is not an admin must already hold everything that account
// will: every grant the stored document names, its row scope, the teams it
// reaches and every field it reads.
//
// The grants come off the STORED document rather than policy.Parse, which
// drops an object this installation cannot name. Such a grant still counts: the
// caller holds nothing on it, so containment cannot be shown and the
// assignment is refused rather than handing out authority an extension's
// return would switch on.
func refuseUnlessCallerMayAssign(ctx context.Context, tx pgx.Tx, actor Identity, roleKey string, raw []byte, teams []ids.TeamID) error {
	if actor.hasRole(roleAdmin) {
		return nil
	}
	if roleKey == roleAdmin {
		return apperrors.ErrPermissionDenied
	}
	stored, err := decodeRoleObjects(raw)
	if err != nil {
		return err
	}
	scope, err := policy.RowScopeOf(raw)
	if err != nil {
		return err
	}
	masks, err := loadFieldMasks(ctx, tx, []string{roleKey})
	if err != nil {
		return err
	}
	objects := make(map[string]principal.ObjectGrant, len(stored))
	for object, grant := range stored {
		objects[object] = principal.ObjectGrant(grant)
	}
	account := seatGrants{
		teams: teams,
		perms: principal.Permissions{Objects: objects, RowScope: scope, FieldMasks: masks},
	}
	if !containsWholeAccess(actor, account) {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// refuseWhileHoldingArchivedRole refuses a verb that would put an account back
// in somebody's hands while it holds an archived role. Restoring the role
// would hand that account its grants again without any assignment check, so
// the member is given a live role first.
func refuseWhileHoldingArchivedRole(ctx context.Context, tx pgx.Tx, userID ids.UserID) error {
	holders, err := archivedRoleHolders(ctx, tx, []ids.UUID{userID.UUID})
	if err != nil {
		return err
	}
	if holders[userID.UUID] {
		return errArchivedRoleHeld
	}
	return nil
}

// archivedRoleHolders reports which of these members hold an archived role.
func archivedRoleHolders(ctx context.Context, tx pgx.Tx, users []ids.UUID) (map[ids.UUID]bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT ra.user_id FROM role_assignment ra JOIN role r ON r.id = ra.role_id
		  WHERE ra.user_id = ANY($1) AND r.archived_at IS NOT NULL`, users)
	if err != nil {
		return nil, err
	}
	holders, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]bool, len(holders))
	for _, holder := range holders {
		out[holder] = true
	}
	return out, nil
}

// lockAuthorization serializes every write that decides, or changes, who may
// do what: role edits, assignments, team membership, deactivation, and the
// credentials a member administrator hands out. Each takes it as the first
// statement of its transaction and checks containment after it, so no role
// can widen between a check and the write it admits. One installation per
// database, so one key.
func lockAuthorization(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('margince:authz')::bigint)`); err != nil {
		return fmt.Errorf("identity: serializing an authorization change: %w", err)
	}
	return nil
}

// administrationObjects are the grants that reach back and change who may do
// what. Handing one out is handing out authority; handing out record work is
// not, which is the line refuseUnlessCallerMayAssign draws.
//
// Derived from policy's own vocabulary rather than listed, so an administration
// object added later is inside this ceiling without anybody remembering to add
// it — the way a hand-kept list silently stops covering the newest authority.
var administrationObjects = policy.AdministrationObjects()
