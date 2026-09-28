// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What the member roster offers a member administrator on each row. Every
// answer comes from the predicates the verbs themselves run — the verb grant,
// the seat, the ceiling over the target, the member's status and kind, the
// last-admin guard and the archived-role hold — so the roster offers what the
// writes accept. The writes still re-ask under their locks: an offer is read
// without them and can go stale before the click.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// memberAction names one verb the roster offers on a member, as the contract
// spells it.
type memberAction string

const (
	actionChangeRole        memberAction = "change_role"
	actionIssuePasswordLink memberAction = "issue_password_link"
	actionDeactivate        memberAction = "deactivate"
	actionReactivate        memberAction = "reactivate"
)

// verb is the user_admin verb the action's endpoint takes. Turning a seat off
// and back on are one authority over one thing; a password link is a
// credential for somebody else's account, so it rides the role-change verb.
// The endpoints spell it as a literal, which the grant-reachability scan reads;
// TestTheRosterOffersOnlyTheVerbsTheCallerHolds holds this to them.
func (a memberAction) verb() principal.Action {
	switch a {
	case actionDeactivate, actionReactivate:
		return principal.ActionDelete
	default:
		return principal.ActionUpdate
	}
}

// memberTarget is what the member verbs decide on about one roster row.
type memberTarget struct {
	row               userRow
	grants            seatGrants
	holdsArchivedRole bool
	soleAdmin         bool
}

// allowedMemberActions answers, per member, the verbs this caller would be
// admitted to right now. Read in the caller's transaction, without the locks
// the writes take.
func (s *Service) allowedMemberActions(ctx context.Context, tx pgx.Tx, actor Identity, rows []userRow) (map[ids.UUID][]memberAction, error) {
	ctx = actorCtx(ctx, actor)
	userIDs := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.ID)
	}
	grants, err := loadGrantsFor(ctx, tx, userIDs)
	if err != nil {
		return nil, err
	}
	archived, err := archivedRoleHolders(ctx, tx, userIDs)
	if err != nil {
		return nil, err
	}
	seatLeft, err := s.fullSeatLeft(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID][]memberAction, len(rows))
	for _, row := range rows {
		target := memberTarget{row: row, grants: grants[row.ID], holdsArchivedRole: archived[row.ID]}
		// soleActiveAdmin answers false for anybody not holding admin, so only
		// those rows spend the query.
		if slices.Contains(row.Roles, roleAdmin) {
			if target.soleAdmin, err = soleActiveAdmin(ctx, tx, ids.UserID{UUID: row.ID}); err != nil {
				return nil, err
			}
		}
		out[row.ID] = memberActionsOn(ctx, actor, target, seatLeft)
	}
	return out, nil
}

// memberActionsOn is the four verbs' admission over one target, in the
// contract's order.
func memberActionsOn(ctx context.Context, actor Identity, target memberTarget, seatLeft bool) []memberAction {
	status, userID := target.row.Status, ids.UserID{UUID: target.row.ID}
	admitted := map[memberAction]bool{
		actionChangeRole: refuseOwnRoleChange(actor, userID) == nil &&
			callerOutranks(actor, target.grants, reachTakeover) == nil &&
			refuseRoleForAgentSeat(target.row.IsAgent) == nil &&
			keepsAnAdmin(target),
		actionIssuePasswordLink: callerOutranks(actor, target.grants, reachTakeover) == nil &&
			!target.holdsArchivedRole &&
			refusePasswordLinkTarget(status, target.row.IsAgent) == nil,
		actionDeactivate: callerOutranks(actor, target.grants, reachDenial) == nil &&
			!alreadyDeactivated(status) &&
			!target.soleAdmin,
		actionReactivate: callerOutranks(actor, target.grants, reachDenial) == nil &&
			!target.holdsArchivedRole &&
			refuseUnlessDeactivated(status) == nil &&
			(target.row.SeatType != principal.SeatFull || seatLeft),
	}
	out := []memberAction{}
	for _, action := range []memberAction{actionChangeRole, actionIssuePasswordLink, actionDeactivate, actionReactivate} {
		if admitted[action] && callerMay(ctx, actor, action) {
			out = append(out, action)
		}
	}
	return out
}

// keepsAnAdmin reports whether some role change leaves the installation an
// administrator. The sole admin holding more roles than admin may still be
// given admin alone, which ChangeUserRole accepts; any other choice for them
// is refused there.
func keepsAnAdmin(target memberTarget) bool {
	return !target.soleAdmin || len(target.row.Roles) > 1
}

// callerMay is the gate in front of every member verb: the user_admin grant
// the action takes, and a seat that may write at all, which the HTTP layer
// checks above RBAC for every mutating method.
func callerMay(ctx context.Context, actor Identity, action memberAction) bool {
	return principal.SeatType(actor.SeatType).CanMutate() &&
		auth.Require(ctx, objectUserAdmin, action.verb()) == nil
}

// alreadyDeactivated reports a member deactivating changes nothing for.
func alreadyDeactivated(status string) bool {
	return status == userStatusDeactivated
}

// refuseUnlessDeactivated refuses reactivating anybody not deactivated. A
// suspended member is held for a different reason (a lockout, say), and this
// path must not quietly clear it.
func refuseUnlessDeactivated(status string) error {
	if status != userStatusDeactivated {
		return errNotDeactivated
	}
	return nil
}

// refuseOwnRoleChange refuses a delegated caller re-roling themselves: that
// passes every containment check trivially, since everybody contains
// themselves, and would hand them record work they lack. The literal admin
// already holds the ceiling, so their own change can only narrow them.
func refuseOwnRoleChange(actor Identity, target ids.UserID) error {
	if target == actor.UserID && !actor.hasRole(roleAdmin) {
		return errOwnRole
	}
	return nil
}

// refuseRoleForAgentSeat refuses a role for the agent seat, which holds none.
func refuseRoleForAgentSeat(isAgent bool) error {
	if isAgent {
		return errAgentSeatHoldsNoRole
	}
	return nil
}
