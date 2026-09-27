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

// refuseUnlessCallerHoldsGrant refuses a role-editor write that turns on a verb
// the caller does not hold.
//
// Needs no transaction: it compares what the write WOULD grant against what the
// caller already holds, and both are values in hand. The stored document is not
// read at all, because what a role currently grants does not bound what this
// caller may add to it — only the caller's own authority does.
func refuseUnlessCallerHoldsGrant(actor Identity, object string, grant storedGrant) error {
	if actor.hasRole(roleAdmin) {
		return nil
	}
	held := actor.Permissions.Objects[object]
	if !held.Contains(principal.ObjectGrant(grant)) {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// roleForAssignment reads the live role a key names, refuses it when this
// caller may not hand it out, and returns its id for the assignment row.
//
// FOR SHARE, so an archive cannot slip between this read and the assignment:
// archiving takes the row FOR UPDATE and then counts holders, so it either
// waits for this transaction and counts the new holder, or commits first and
// this read finds no live role. An archived role answers errUnknownRole — it is
// not a role anybody can be given.
func roleForAssignment(ctx context.Context, tx pgx.Tx, actor Identity, roleKey string) (ids.UUID, error) {
	var roleID ids.UUID
	var raw []byte
	err := tx.QueryRow(ctx,
		`SELECT id, permissions FROM role WHERE key = $1 AND archived_at IS NULL FOR SHARE`,
		roleKey).Scan(&roleID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, errUnknownRole
	}
	if err != nil {
		return ids.UUID{}, err
	}
	if err := refuseUnlessCallerMayAssign(actor, roleKey, raw); err != nil {
		return ids.UUID{}, err
	}
	return roleID, nil
}

// refuseUnlessCallerMayAssign answers the other half: may this caller hand out
// THIS role. A delegated holder assigning `admin` — or a custom role carrying
// the role editor, or the installation reset — reaches the same takeover by a
// longer path than a password link.
//
// The literal admin role requires the literal admin role. Every other key is
// checked by containment over the administration grants and the row scope, so
// nobody hands out authority they do not have.
func refuseUnlessCallerMayAssign(actor Identity, roleKey string, raw []byte) error {
	if actor.hasRole(roleAdmin) {
		return nil
	}
	if roleKey == roleAdmin {
		return apperrors.ErrPermissionDenied
	}
	// Resolved through the same Parse/Merge the login path uses, so what the
	// role WOULD confer on its holder is what is compared. A second decoder here
	// would be a second answer to "what does this document grant", and the two
	// would disagree the first time Parse's vocabulary changed.
	doc, err := policy.Parse(raw)
	if err != nil {
		return err
	}
	would := policy.Merge(map[string]policy.Document{roleKey: doc})

	// Containment over the ADMINISTRATION objects, not over every object.
	//
	// Full containment was the first rule here and it was wrong in the direction
	// that matters: a member administrator delegated onboarding holds user_admin
	// and little else, so requiring them to hold deal.create and contact.update
	// before they could invite a rep refused the exact delegation this change
	// exists to enable. An admit test caught it; the refusal arms alone would
	// have passed against a guard that refused everybody.
	//
	// What escalates is handing out AUTHORITY, and authority is what these
	// objects name. A role carrying role_admin or system_reset reaches back and
	// rewrites who may do what, so a caller must already hold it before they may
	// hand it out. Row scope escalates exactly like a grant: a caller bounded to
	// their team must not hand out a role that reads every row.
	if !containsAdministration(actor.Permissions, would) {
		return apperrors.ErrPermissionDenied
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
