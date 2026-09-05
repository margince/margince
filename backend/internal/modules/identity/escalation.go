// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Who may act on WHOM, once administering members is a grant rather than a role.
//
// While `user_admin`'s verbs were reachable only by an admin, no call site had
// to read the target's role: every caller already held every authority every
// target could hold, so there was nothing to escalate to. Handing the grant to a
// non-admin removes that guarantee and opens three takeover paths, all of which
// end at the same place — an admin account under someone else's control:
//
//   - IssuePasswordLink mints a single-use password token for any user and
//     returns the raw value. A delegated holder could take over an admin
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
// grant itself admin-equivalence would make the ceiling meaningless.
//
// Read inside the caller's transaction, never before it. A role assignment
// changed between the check and the write would otherwise decide the answer
// against a state that no longer holds.

import (
	"context"
	"errors"

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

// targetRoleKeys reads the roles a user holds, locking the USER row first so the
// set cannot change under the mutation that follows.
//
// The lock is on app_user and not on role_assignment, and the difference is load
// bearing. A row lock at READ COMMITTED locks rows that EXIST; there is no gap
// lock, so `FOR UPDATE` over the assignments of a target who currently holds
// none locks nothing at all. ChangeUserRole re-roles by DELETE-then-INSERT, so
// every target passes through exactly that state mid-transaction — and a guard
// reading it there would see no keys, admit the caller, and let the write land
// against a user who commits as an admin a moment later.
//
// app_user is the one row every writer of a role assignment can agree on, and
// DeactivateUser already locks it. Taking it here makes the read and the
// re-roling serial on a row that is always there.
//
// A user with no assignment is not an error: an agent seat holds no role, and
// neither does a member whose last role was removed. Both are targets a caller
// may act on, because an empty authority is contained by every authority.
func targetRoleKeys(ctx context.Context, tx pgx.Tx, userID ids.UserID) ([]string, error) {
	var exists bool
	switch err := tx.QueryRow(ctx,
		`SELECT true FROM app_user WHERE id = $1 AND archived_at IS NULL FOR UPDATE`,
		userID).Scan(&exists); {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, apperrors.ErrNotFound
	case err != nil:
		return nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT r.key FROM role_assignment ra JOIN role r ON r.id = ra.role_id
		 WHERE ra.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if scanErr := rows.Scan(&key); scanErr != nil {
			return nil, scanErr
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// refuseUnlessCallerOutranksTarget is the guard every delegated member verb
// carries. It answers one question: may this caller act on this target at all.
//
// Only the literal admin role is checked today, and that is the whole ceiling
// rather than a first approximation. The seeded roles below admin hold no
// authority admin lacks, so containment reduces to "is the target an admin",
// and a custom role wide enough to matter can only be created BY an admin
// through the role editor — which carries its own floor.
//
// Returns ErrPermissionDenied and not ErrNotFound: the target's existence is
// already disclosed by the roster this caller may read, so hiding it here would
// buy nothing and would tell an operator their own action failed for the wrong
// reason.
func refuseUnlessCallerOutranksTarget(ctx context.Context, tx pgx.Tx, actor Identity, targetID ids.UserID) error {
	if actor.hasRole(roleAdmin) {
		return nil
	}
	keys, err := targetRoleKeys(ctx, tx, targetID)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if key == roleAdmin {
			return apperrors.ErrPermissionDenied
		}
	}
	return nil
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

// refuseUnlessCallerMayAssign answers the other half: may this caller hand out
// THIS role. A delegated holder assigning `admin` — or a custom role carrying
// the role editor, or the installation reset — reaches the same takeover by a
// longer path than a password link.
//
// The literal admin role requires the literal admin role. Every other key is
// checked by containment: the caller must already hold every object grant the
// role would confer, so nobody hands out authority they do not have.
func refuseUnlessCallerMayAssign(ctx context.Context, tx pgx.Tx, actor Identity, roleKey string) error {
	if actor.hasRole(roleAdmin) {
		return nil
	}
	if roleKey == roleAdmin {
		return apperrors.ErrPermissionDenied
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT permissions FROM role WHERE key = $1`, roleKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
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
	// and little else, so requiring them to hold deal.create and person.update
	// before they could invite a rep refused the exact delegation this change
	// exists to enable. An admit test caught it; the refusal arms alone would
	// have passed against a guard that refused everybody.
	//
	// What escalates is handing out AUTHORITY, and authority is what these
	// objects name. A rep's grid is record work — a caller who cannot work deals
	// gains nothing by creating somebody who can, because the new account is not
	// theirs to act as. A role carrying role_admin or system_reset is different
	// in kind: it reaches back and rewrites who may do what, so a caller must
	// already hold it before they may hand it out.
	for _, object := range administrationObjects {
		if !actor.Permissions.Objects[object].Contains(would.Objects[object]) {
			return apperrors.ErrPermissionDenied
		}
	}
	// Row scope is the third axis and escalates exactly like a grant: a caller
	// bounded to their team must not hand out a role that reads every row.
	if would.RowScope.Wider(actor.Permissions.RowScope) {
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
