// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The authority an automatic apply acts under.
//
// A reversible correction the product is confident about applies itself rather
// than waiting in somebody's queue, and the question this file answers is whose
// authority it acts on. Not the system principal: auth.Require passes it
// unconditionally, no row scope bounds it, and approvals/decidingactor.go
// refuses it outright — approvals are decided by people. A change applied under
// an authority nobody holds is exactly the change nobody can be shown to have
// been allowed to make.
//
// So an automatic apply runs as an AGENT acting for the record's current owner:
// the same shape a passport carries, which actingForAHuman already admits and
// which the stores' own RBAC and row-scope gates already bound. The audit row
// then says both halves — actor_type=agent, on_behalf_of=the owner — so the
// history reads "Margince, for Anna Weber" rather than crediting Anna with a
// change she was asleep for, or crediting nobody at all.
//
// The owner is resolved at APPLY time from the record, never carried on the
// staged row. A handover between staging and applying moves who it acts for,
// which is the honest answer: the person who holds the account now is the person
// whose authority the change is made under.
//
// Three states refuse, and all three leave the proposal in the queue for a
// person rather than applying it more weakly:
//   - the record has no owner (owner_id is nullable, ON DELETE SET NULL)
//   - the owner is suspended, archived or removed (EffectiveAuthority answers
//     ErrNotFound; absence of authority is denial, never empty permission)
//   - the owner may no longer write the record, which the store's own gate says
//
// Refusing is not a failure of the pass. It is the pass declining to act where
// it cannot name an authority, and the card is still there for whoever can.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// errNoRecordOwner reports a record nobody owns. A real state, not a fault:
// there is no authority to apply under, so the caller leaves the proposal be.
var errNoRecordOwner = errors.New("compose: the record has no owner to apply as")

// errOwnerHasNoAuthority reports an owner who can no longer act — suspended,
// archived, removed, or stripped of the grant the change needs.
var errOwnerHasNoAuthority = errors.New("compose: the record's owner holds no authority to apply under")

// errUnownableTarget reports a target type no owner can be read from.
//
// A sentinel rather than a formatted string because a test must be able to tell
// this refusal from the database error that follows it when the guard is gone —
// both name the target type, so matching on the message proves nothing.
var errUnownableTarget = errors.New("compose: auto-apply cannot resolve an owner for this target type")

// autoApplyAuthority answers "apply this change as the record's owner would,
// and say that the product did it".
//
// It shares dealOwnerAuthority's question and not its answer. That seam binds a
// PrincipalHuman because it exists to READ as the owner, and a read wants the
// owner's own eyes. This one writes, and a write credited to a human who did not
// make it is the audit trail telling a reader something false.
type autoApplyAuthority struct {
	db    *database.DB
	users *identity.Service
}

// ownerColumns names the table each approval target type keeps its owner in.
//
// A map rather than a switch so the set is enumerable: the auto-apply census
// asks it which target types it can resolve, and a kind whose target is missing
// here is a kind that cannot apply itself. Both tables are compile-time
// literals — no identifier reaches the statement from a request body.
var ownerColumns = map[string]string{
	"deal":         "SELECT owner_id FROM deal WHERE id = $1",
	"organization": "SELECT owner_id FROM organization WHERE id = $1",
}

// resolvableTarget reports whether an automatic apply can name an authority for
// this target type at all.
func resolvableTarget(targetType string) bool {
	_, ok := ownerColumns[targetType]
	return ok
}

// contextFor binds the record's owner as the acting authority, as an agent.
//
// Returns errNoRecordOwner for an unowned record and errOwnerHasNoAuthority for
// an owner who cannot act. Both are refusals the caller reports and moves past;
// neither is an error that should fail a pass over other records.
func (a autoApplyAuthority) contextFor(ctx context.Context, targetType string, id ids.UUID) (context.Context, error) {
	owner, err := a.recordOwner(ctx, targetType, id)
	if err != nil {
		return nil, err
	}
	if owner.IsZero() {
		return nil, errNoRecordOwner
	}
	return a.asAgentFor(ctx, owner)
}

// recordOwner reads who the record belongs to. It runs under the CALLER's own
// principal — the sweep's — because owner_id is what CHOOSES the acting
// authority, and a read that needed that authority to find it could not start.
func (a autoApplyAuthority) recordOwner(ctx context.Context, targetType string, id ids.UUID) (ids.UUID, error) {
	statement, ok := ownerColumns[targetType]
	if !ok {
		return ids.UUID{}, fmt.Errorf("%w: %q", errUnownableTarget, targetType)
	}
	var owner ids.UUID
	err := a.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, statement, id).Scan(&owner)
	})
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: reading the %s owner to apply as: %w", targetType, err)
	}
	return owner, nil
}

// asAgentFor binds the product as an agent acting for one person.
//
// The principal carries the owner's real grants, resolved in ONE snapshot, so
// the apply is bounded by exactly what that person could do by hand. Two fields
// carry the rest of the meaning:
//
//   - PassportID stays nil. There is no credential here, and the
//     proposer-does-not-confirm rule in agentMayDecide keys on a passport id
//     matching the staged row's — nil matches nothing, which is correct: the
//     product proposing and the product applying is the whole point, and what
//     that rule guards is a CREDENTIAL walking through its own tier.
//   - ScopeWrite, and nothing else. agentMayDecide charges the write cap on any
//     decision; the send cap it charges for a sending kind is deliberately
//     absent, so an auto-apply that ever reached one would be refused there
//     rather than here. Two gates saying no is the shape worth having.
func (a autoApplyAuthority) asAgentFor(ctx context.Context, owner ids.UUID) (context.Context, error) {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, errors.New("compose: applying as a record owner outside a bound workspace")
	}
	rbac, seat, err := a.users.EffectiveAuthority(ctx, wsID, owner)
	if err != nil {
		return nil, fmt.Errorf("%w: owner %s: %w", errOwnerHasNoAuthority, owner, err)
	}
	return principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalAgent,
		ID:          autoApplyActorID,
		UserID:      owner,
		OnBehalfOf:  owner,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
		Scopes:      principal.NewScopeSet(principal.ScopeWrite),
	}), nil
}

// autoApplyActorID names the product in an audit row's actor_id, so a reader
// can tell an automatic apply from a passport an agent holds. It is not a
// credential and nothing mints it.
const autoApplyActorID = "agent:auto-apply"
