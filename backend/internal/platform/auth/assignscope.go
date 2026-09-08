// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AssigneeNotAllowedError refuses a proposed owner: 422, one code for every
// reason.
//
// One code on purpose. "Not a user here", "suspended", "an agent", "a read
// seat" and "on a team you do not lead" are five different facts, and telling
// them apart would let a rep enumerate the roster and read the team graph off
// the refusals. The caller's remedy is the same in all five — pick somebody the
// picker offered — so the sentence names the rule rather than the miss.
type AssigneeNotAllowedError struct{}

func (e *AssigneeNotAllowedError) Error() string {
	return "the owner must be an active colleague you may assign work to"
}

// FieldFault names owner_id, the field the caller actually sent.
func (e *AssigneeNotAllowedError) FieldFault() (field, code, message string) {
	return "owner_id", "owner_not_assignable", e.Error()
}

// assigneeEligible is the destination arm, and it is deliberately NOT the
// write arm.
//
// A seat may be handed work when it is a live human seat that can do the work —
// active, unarchived, not an agent, and holding a seat that can write — AND it
// falls inside the caller's own write scope. The scope half reuses
// ownerPredicate so the rule is spelled once: after the assignment lands, the
// row sits inside the write scope the caller already had, which is what makes
// "assign" incapable of pushing a record somewhere its assigner cannot follow.
//
// Held by: TestSeatEligibilityIsAskedTheSameWayEverywhereItIsAsked (backend/gates/assigneeeligibility_test.go)
//
// The grant arm of writeAuthorityPredicateAs is deliberately absent. A share
// names ONE record; it says who may change that row, never who may receive
// other rows. Reading it here would let a single write share on one lead widen
// the assigner's whole destination pool.
func assigneeEligible(p principal.Principal, alias string, arg func(any) int) string {
	return fmt.Sprintf(
		`%[1]s.status = 'active' AND %[1]s.archived_at IS NULL
		   AND NOT %[1]s.is_agent AND %[1]s.seat_type <> 'read'
		   AND %[2]s`,
		alias, ownerPredicate(p, arg, unownedIsNobodys)(alias))
}

// EnsureAssignee answers whether the caller may hand work to dest.
//
// The scope question is asked of a HYPOTHETICAL row: the app_user row is
// selected with its own id aliased as owner_id, so the write-scope owner
// predicate — which knows only how to read an owner_id — answers "would a row
// owned by dest be mine to change?". That is the same predicate the write path
// runs, rather than a second reading of the same rule that could drift from it.
//
// An unbounded seat still passes through the eligibility half: admin may assign
// to anyone, and "anyone" has never included a suspended seat or an agent.
func EnsureAssignee(ctx context.Context, tx pgx.Tx, dest ids.UUID) error {
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	destPos := arg(dest)

	var permitted bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM (
		   SELECT id AS owner_id, status, archived_at, is_agent, seat_type
		   FROM app_user WHERE id = $%d) u WHERE %s)`,
		destPos, assigneeEligible(p, "u", arg)), args...).Scan(&permitted); err != nil {
		return err
	}
	return refuseIneligibleAssignee(permitted)
}

// refuseIneligibleAssignee turns the eligibility answer into the refusal.
//
// Its own named helper so the auth package's own census can see that
// EnsureAssignee refuses: the refusal is a TYPED error carrying a field fault
// (422 naming owner_id), not one of the shared sentinels the census greps for,
// and a gate whose refusal nothing recognizes is one nobody notices going
// quiet.
func refuseIneligibleAssignee(permitted bool) error {
	if permitted {
		return nil
	}
	return &AssigneeNotAllowedError{}
}

// EnsureAssignable is the gate in front of handing a row to somebody else.
//
// Two halves, and both must hold. The SOURCE half asks whether this row is the
// caller's to hand on: theirs to change already, or ownerless — the same door
// EnsureClaimable opens, because a record nobody owns is exactly the one a
// manager is supposed to be able to route without claiming it first. The
// DESTINATION half is EnsureAssignee.
//
// Splitting them is what makes the refusals honest: a source the caller may not
// touch is 403/404 through the write arm's own contracts, while a destination
// they may not assign to is 422 naming owner_id. Collapsing the two would
// report "you may not edit this lead" for a perfectly writable lead handed to a
// suspended colleague.
//
// Human-only on the ownerless arm, matching ClaimRecord. Putting a name on a
// record nobody owns is a person's act; an agent may still hand on a row its
// delegated authority already covers.
func EnsureAssignable(ctx context.Context, tx pgx.Tx, table string, id, dest ids.UUID) error {
	if err := ensureAssignableSource(ctx, tx, table, id); err != nil {
		return err
	}
	return EnsureAssignee(ctx, tx, dest)
}

// ensureAssignableSource is EnsureAssignable's source half: writable already,
// or ownerless and the caller is a human.
func ensureAssignableSource(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if !ownerScopedTables[table] {
		return fmt.Errorf("auth: %q is not a row-scoped table", table)
	}
	if err := EnsureVisibleLive(ctx, tx, table, id); err != nil {
		return err
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if Unbounded(p) {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)
	clause := writeAuthorityPredicate(p, table, arg)

	// Two answers, not one: whether the row is writable, and whether it is
	// ownerless. A single EXISTS over the OR would refuse an agent on an
	// ownerless row with the same silence as a row that is somebody else's,
	// and the ownerless arm is the one that carries the human-only rule.
	var writable, unowned bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT COALESCE(bool_or(%[3]s), false), COALESCE(bool_or(%[1]s.owner_id IS NULL), false)
		   FROM %[1]s WHERE id = $%[2]d`, table, idPos, clause),
		args...).Scan(&writable, &unowned); err != nil {
		return err
	}
	if writable {
		return nil
	}
	if !unowned {
		return apperrors.ErrPermissionDenied
	}
	// RequireHuman rather than a type comparison: it also refuses a buyer
	// principal, which is a seat the raw check would have admitted onto the
	// ownerless arm.
	return RequireHuman(ctx)
}
