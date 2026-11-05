// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
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

// AssigneeEligibleSQL is the destination arm: an active, unarchived human seat
// that can write. The caller's write scope is not asked, or an own-scope seat
// could hand work to nobody but itself.
//
// The source half (EnsureAssignable, or the update's write arm) keeps a caller
// to the records they may change.
// Held by: TestSeatEligibilityIsAskedTheSameWayEverywhereItIsAsked (backend/gates/assigneeeligibility_test.go)
func AssigneeEligibleSQL(alias string) string {
	return fmt.Sprintf(
		`%[1]s
		   AND NOT %[2]s.is_agent AND %[2]s.seat_type <> 'read'`,
		workforce.LiveMemberSQL(alias), alias)
}

// runAsOwnerSQL is AssigneeEligibleSQL for an owner a record RUNS AS, such as
// an automation or a webhook. Naming that owner is acting for them, so they
// must also sit inside the caller's write scope. A share is not read: it names
// one row, never whom the caller may act for.
func runAsOwnerSQL(p principal.Principal, alias string, arg func(any) int) string {
	return AssigneeEligibleSQL(alias) + " AND " + ownerPredicate(p, arg, unownedIsNobodys)(alias)
}

// recordOwnerSQL is AssigneeEligibleSQL in the shape ensureAssigneeWith asks.
func recordOwnerSQL(_ principal.Principal, alias string, _ func(any) int) string {
	return AssigneeEligibleSQL(alias)
}

// newRecordOwnerSQL is AssigneeEligibleSQL for a NEW record's named owner,
// where an invited colleague is also eligible. An invited colleague is a real
// colleague who has not signed in yet, and naming them as the owner of a record
// created on their behalf (an import, most of all) is the act the assignments
// module already allows for responsibilities. Routing (round robin, SLA
// reassignment, task fan-out) keeps asking AssigneeEligibleSQL, so nothing is
// handed automatically to a seat nobody answers.
//
// Derived from AssigneeEligibleSQL rather than spelled again, so every other
// clause stays the one rule; TestANewRecordMayNameAnInvitedOwner holds the
// swap.
func newRecordOwnerSQL(_ principal.Principal, alias string, _ func(any) int) string {
	return strings.Replace(AssigneeEligibleSQL(alias),
		workforce.LiveMemberSQL(alias), workforce.ActivatableMemberSQL(alias), 1)
}

// EnsureAssignee answers whether the caller may hand a record to dest: a live
// human seat that can write. An unbounded seat asks the same, because "anyone"
// has never included a suspended seat or an agent.
func EnsureAssignee(ctx context.Context, tx pgx.Tx, dest ids.UUID) error {
	return ensureAssigneeWith(ctx, tx, dest, recordOwnerSQL)
}

// EnsureRunAsOwner is EnsureAssignee for an owner the record runs as
// (runAsOwnerSQL): dest must also fall inside the caller's own write scope.
//
// The scope is asked of a HYPOTHETICAL row: app_user's id is aliased as
// owner_id, so the owner predicate answers "would dest's row be mine?".
func EnsureRunAsOwner(ctx context.Context, tx pgx.Tx, dest ids.UUID) error {
	return ensureAssigneeWith(ctx, tx, dest, runAsOwnerSQL)
}

// EligibleAssignees answers EnsureAssignee for many seats in one query. It
// returns the candidates the caller may make an owner, so a picker built on it
// offers what the write accepts.
func EligibleAssignees(ctx context.Context, tx pgx.Tx, candidates []ids.UUID) (map[ids.UUID]bool, error) {
	eligible := make(map[ids.UUID]bool, len(candidates))
	if len(candidates) == 0 {
		return eligible, nil
	}
	query := fmt.Sprintf(
		`SELECT u.owner_id FROM (
		   SELECT id AS owner_id, status, archived_at, is_agent, seat_type
		   FROM app_user WHERE id = ANY($1)) u WHERE %s`,
		AssigneeEligibleSQL("u"))
	rows, err := tx.Query(ctx, query, candidates)
	if err != nil {
		return nil, fmt.Errorf("ask which seats may own records: %w", err)
	}
	admitted, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("ask which seats may own records: %w", err)
	}
	for _, id := range admitted {
		eligible[id] = true
	}
	return eligible, nil
}

// EnsureNewRecordOwner is EnsureAssignee for the owner named on a record's
// create, where an invited colleague is also eligible (newRecordOwnerSQL).
func EnsureNewRecordOwner(ctx context.Context, tx pgx.Tx, dest ids.UUID) error {
	return ensureAssigneeWith(ctx, tx, dest, newRecordOwnerSQL)
}

func ensureAssigneeWith(ctx context.Context, tx pgx.Tx, dest ids.UUID,
	eligible func(principal.Principal, string, func(any) int) string,
) error {
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	destPos := arg(dest)

	// An owner a case admission names already holds a merged contact, and a
	// consented receiver agreed to take the record. So the run-as scope is not
	// asked of them, and an invited seat is not admitted.
	if admittedCaseGrantee(ctx, dest) || handOverConsented(ctx, dest) {
		eligible = recordOwnerSQL
	}
	status := "status"
	if consolidatingInto(ctx, dest) {
		if err := Require(ctx, "user_admin", principal.ActionDelete); err != nil {
			return err
		}
		status = "CASE WHEN status IN ('invited','deactivated') THEN 'active' ELSE status END AS status"
	}
	var permitted bool
	query := fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM (
		   SELECT id AS owner_id, %s, archived_at, is_agent, seat_type
		   FROM app_user WHERE id = $%d AND removed_at IS NULL FOR KEY SHARE) u WHERE %s)`,
		status, destPos, eligible(p, "u", arg))
	if err := tx.QueryRow(ctx, query, args...).Scan(&permitted); err != nil {
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
// record nobody owns is a contact's act; an agent may still hand on a row its
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

// EnsureOwnerHandOn is EnsureAssignee for an EDIT that names an owner, where
// only a CHANGE of owner is an assignment.
//
// An edit form sends the whole record back, owner included, so the ordinary
// save of a record whose owner has since been suspended re-states an owner it
// is not handing on. Asking EnsureAssignee of that would make such a record
// uneditable by anyone until somebody first handed it elsewhere — the record a
// departed colleague leaves behind, frozen exactly when it needs picking up.
func EnsureOwnerHandOn(ctx context.Context, tx pgx.Tx, current *ids.UUID, next *ids.UserID) error {
	return ensureHandOnWith(ctx, tx, current, next, EnsureAssignee)
}

// EnsureRunAsOwnerHandOn is EnsureOwnerHandOn for an owner the record runs as,
// asking EnsureRunAsOwner of a changed owner.
func EnsureRunAsOwnerHandOn(ctx context.Context, tx pgx.Tx, current *ids.UUID, next *ids.UserID) error {
	return ensureHandOnWith(ctx, tx, current, next, EnsureRunAsOwner)
}

func ensureHandOnWith(ctx context.Context, tx pgx.Tx, current *ids.UUID, next *ids.UserID,
	ensure func(context.Context, pgx.Tx, ids.UUID) error,
) error {
	if next == nil {
		return nil
	}
	if current != nil && *current == next.UUID {
		return nil
	}
	return ensure(ctx, tx, next.UUID)
}
