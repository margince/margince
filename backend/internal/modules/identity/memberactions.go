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
	actionEdit              memberAction = "edit"
	actionMerge             memberAction = "merge"
	actionRemove            memberAction = "remove"
	actionChangeRole        memberAction = "change_role"
	actionIssuePasswordLink memberAction = "issue_password_link"
	actionResendInvite      memberAction = "resend_invite"
	actionRemoveMFA         memberAction = "remove_mfa"
	actionDeactivate        memberAction = "deactivate"
	actionReactivate        memberAction = "reactivate"
	actionHandOver          memberAction = "hand_over"
)

// verb is the user_admin verb the action's endpoint takes. Turning a seat off
// and back on are one authority over one thing. A password link is a
// credential for somebody else's account. Removing a second factor weakens
// one. So both ride the role-change verb.
// The endpoints spell it as a literal, which the grant-reachability scan reads;
// TestTheRosterOffersOnlyTheVerbsTheCallerHolds holds this to them.
func (a memberAction) verb() principal.Action {
	switch a {
	case actionDeactivate, actionReactivate, actionMerge, actionRemove:
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
	hasFactor         bool
}

// memberOffer is what the roster row says about the verbs: those admitted and
// those the caller holds but the member refuses.
type memberOffer struct {
	Allowed []memberAction
	Refused []memberRefusal
	// MergeNeedsConfirmation is set when merge is offered on a used account,
	// whose merge waits for the holders to confirm.
	MergeNeedsConfirmation bool
}

// allowedMemberActions answers, per member, the verbs this caller would be
// admitted to right now. Read in the caller's transaction, without the locks
// the writes take.
func (s *Service) allowedMemberActions(
	ctx context.Context, tx pgx.Tx, actor Identity, rows []userRow, grants map[ids.UUID]seatGrants,
) (map[ids.UUID]memberOffer, error) {
	ctx = actorCtx(ctx, actor)
	archived, err := archivedRoleHolders(ctx, tx, rowIDs(rows))
	if err != nil {
		return nil, err
	}
	factors, err := confirmedFactorHolders(ctx, tx, rowIDs(rows))
	if err != nil {
		return nil, err
	}
	seatLeft, err := s.fullSeatLeft(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]memberOffer, len(rows))
	for _, row := range rows {
		target := memberTarget{
			row: row, grants: grants[row.ID], holdsArchivedRole: archived[row.ID], hasFactor: factors[row.ID],
		}
		// soleActiveAdmin answers false for anybody not holding admin, so only
		// those rows spend the query.
		if slices.Contains(row.Roles, roleAdmin) {
			if target.soleAdmin, err = soleActiveAdmin(ctx, tx, ids.UserID{UUID: row.ID}); err != nil {
				return nil, err
			}
		}
		allowed := memberActionsOn(ctx, actor, target, seatLeft)
		out[row.ID] = memberOffer{
			Allowed: allowed, Refused: refusedMemberActions(ctx, actor, target),
			MergeNeedsConfirmation: slices.Contains(allowed, actionMerge) && !row.Usage.unused(),
		}
	}
	return out, nil
}

// memberActionsOn is the six verbs' admission over one target, in the
// contract's order.
func memberActionsOn(ctx context.Context, actor Identity, target memberTarget, seatLeft bool) []memberAction {
	status, userID := target.row.Status, ids.UserID{UUID: target.row.ID}
	admitted := map[memberAction]bool{
		actionEdit:  !target.row.IsAgent && callerOutranks(actor, target.grants, reachTakeover) == nil,
		actionMerge: actor.hasRole(roleAdmin) && len(mergeRefusals(actor, target)) == 0,
		// Removal asks for a deactivated member, as the retirement preview does.
		actionRemove: actor.hasRole(roleAdmin) && !target.row.IsAgent && userID != actor.UserID && !target.soleAdmin &&
			refuseUnlessDeactivated(status) == nil,
		actionChangeRole: refuseOwnRoleChange(actor, userID) == nil &&
			callerOutranks(actor, target.grants, reachTakeover) == nil &&
			refuseRoleForAgentSeat(target.row.IsAgent) == nil &&
			keepsAnAdmin(target),
		actionIssuePasswordLink: callerOutranks(actor, target.grants, reachTakeover) == nil &&
			!target.holdsArchivedRole &&
			refusePasswordLinkTarget(status, target.row.IsAgent) == nil,
		actionResendInvite: callerOutranks(actor, target.grants, reachTakeover) == nil &&
			!target.holdsArchivedRole && refuseResendTarget(status, target.row.IsAgent) == nil,
		actionRemoveMFA: target.hasFactor && userID != actor.UserID && !target.holdsArchivedRole &&
			callerOutranks(actor, target.grants, reachTakeover) == nil,
		actionDeactivate: callerOutranks(actor, target.grants, reachDenial) == nil &&
			!alreadyDeactivated(status) &&
			!target.soleAdmin,
		actionReactivate: callerOutranks(actor, target.grants, reachDenial) == nil &&
			!target.holdsArchivedRole &&
			refuseUnlessDeactivated(status) == nil &&
			(target.row.SeatType != principal.SeatFull || seatLeft),
		// AdmitMemberHandover is the handover's gate; this is its offer.
		actionHandOver: !target.row.IsAgent && callerOutranks(actor, target.grants, reachDenial) == nil,
	}
	out := []memberAction{}
	for _, action := range []memberAction{
		actionChangeRole, actionIssuePasswordLink, actionResendInvite, actionRemoveMFA, actionDeactivate, actionReactivate, actionHandOver, actionEdit, actionMerge, actionRemove,
	} {
		if admitted[action] && callerMay(ctx, actor, action) {
			out = append(out, action)
		}
	}
	return out
}

// memberRefusal is a verb the caller holds the grant for and the target
// refuses, with the reason the roster shows in place of the button.
type memberRefusal struct {
	Action memberAction
	Reason string
}

// The reasons a merge is refused, as the contract spells them.
const (
	reasonSoleAdmin = "sole_admin"
	reasonSelf      = "self"
	reasonAgentSeat = "agent_seat"
)

// mergeRefusals lists every reason this target cannot be merged away by this
// caller, in the order the roster shows them. Empty means merge is admitted.
// A used account is admitted: its merge waits for the holders' confirmation,
// which the preview asks for, so use is no refusal.
// The merge offer is decided here alone, so a reason shown is always a reason
// the offer lacks.
func mergeRefusals(actor Identity, target memberTarget) []string {
	var out []string
	if target.row.IsAgent {
		return append(out, reasonAgentSeat)
	}
	if target.row.ID == actor.UserID.UUID {
		return append(out, reasonSelf)
	}
	if target.soleAdmin {
		out = append(out, reasonSoleAdmin)
	}
	return out
}

// refusedMemberActions names what the caller may do in principle on this
// member and may not right now. Only an administrator sees merge, and only
// with the grant, so a refusal never discloses a verb the caller lacks.
func refusedMemberActions(ctx context.Context, actor Identity, target memberTarget) []memberRefusal {
	if !actor.hasRole(roleAdmin) || !callerMay(ctx, actor, actionMerge) {
		return nil
	}
	var out []memberRefusal
	for _, reason := range mergeRefusals(actor, target) {
		out = append(out, memberRefusal{Action: actionMerge, Reason: reason})
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

// callerMay decides whether a member verb is offered: the user_admin grant the
// action takes, and a seat that may write at all. The write handlers enforce it.
func callerMay(ctx context.Context, actor Identity, action memberAction) bool {
	return principal.SeatType(actor.SeatType).CanMutate() &&
		auth.Allows(ctx, objectUserAdmin, action.verb())
}

// alreadyDeactivated reports a member deactivating changes nothing for.
func alreadyDeactivated(status string) bool {
	return status == userStatusDeactivated
}

// refuseUnlessDeactivated refuses reactivating anybody not deactivated. A
// suspended member is held for a different reason (a lockout, say), and this
// path must not quietly clear it.
func refuseUnlessDeactivated(status string) error {
	if !alreadyDeactivated(status) {
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
