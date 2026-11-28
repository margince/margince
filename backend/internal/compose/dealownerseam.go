// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading a deal's world as the contact the deal belongs to.
//
// A nightly sweep runs as a system principal, which auth.Require passes
// unconditionally and which no row scope bounds. Its writes to an owned deal
// run as the owner's agent instead (agentForTx). The system principal is
// also wrong for anything the sweep reads and then STORES into an approval
// payload. Whoever holds the card's decide grant reads that payload later, and
// that grant set is narrower than the principal that filled it. A string resolved under the system principal and frozen into a row
// is a disclosure no read-side gate can undo.
//
// So a producer that puts human-identifying data or message content on a card
// resolves it under the deal owner's own authority instead. Two producers do:
// the close-date review names who went quiet, and the overnight drafter puts a
// counterparty's address and the message it answers into a draft. They share
// this seam rather than each carrying a copy — the rule is one rule, and a
// second spelling of it would be the one that stops matching.
//
// The owner is the right authority because the card is FOR them: it lands in
// their queue, about their deal. Nothing here widens what they may see.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/carddoer"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// dealOwnerAuthority answers "read this deal's world as its owner would".
type dealOwnerAuthority struct {
	db    *database.DB
	users *identity.Service
}

// errNoDealOwner reports a deal nobody owns, which is a real state rather than
// a failure: owner_id is nullable and ON DELETE SET NULL clears it when a member
// is removed. There is no authority to read under, so the caller degrades —
// it does not fail the pass over every other deal in the workspace.
var errNoDealOwner = errors.New("compose: the deal has no owner to read as")

// contextFor binds the deal's owner as the acting principal.
//
// Returns errNoDealOwner for an unowned deal, and wraps apperrors.ErrNotFound
// from EffectiveAuthority for an owner who is suspended, archived or removed —
// absence of authority is denial, never empty permission.
func (a dealOwnerAuthority) contextFor(ctx context.Context, dealID ids.UUID) (context.Context, error) {
	owner, err := a.dealOwner(ctx, dealID)
	if err != nil {
		return nil, err
	}
	if owner.IsZero() {
		return nil, errNoDealOwner
	}
	return a.asOwner(ctx, owner)
}

// dealOwner reads who the deal belongs to. It runs under the CALLER's own
// principal — the sweep's — because owner_id is what chooses the reading
// authority, and a read that needed the authority to find it could not start.
func (a dealOwnerAuthority) dealOwner(ctx context.Context, dealID ids.UUID) (ids.UUID, error) {
	var owner ids.UUID
	err := a.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		owner, err = lockedDealOwner(ctx, tx, dealID)
		return err
	})
	return owner, err
}

// lockedDealOwner reads the owner and holds it until the caller's transaction
// ends. A reassignment waits behind the lock, so whatever the caller stages
// here names the owner that is current when it commits; one that committed
// first hands its proposals on itself (approvals.FollowDealOwnerInTx).
func lockedDealOwner(ctx context.Context, tx pgx.Tx, dealID ids.UUID) (ids.UUID, error) {
	var owner *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM deal WHERE id = $1 FOR SHARE`, dealID).Scan(&owner); err != nil {
		return ids.Nil, fmt.Errorf("compose: deal %s owner: %w", dealID, err)
	}
	if owner == nil {
		return ids.Nil, nil
	}
	return *owner, nil
}

// colleagueAuthority reads as any one colleague, for a card that hands work to
// them, through the same snapshot asOwner takes.
func colleagueAuthority(db *database.DB) carddoer.ColleagueAuthority {
	return dealOwnerAuthority{db: db, users: identity.NewServiceFor(db)}.asOwner
}

// asOwner binds one member as the acting principal.
//
// EffectiveAuthority reads the grants and the seat as ONE snapshot: composed
// from separate reads they can describe an authority the member never held —
// permissions from before a role change with a seat from after. SeatType is
// carried rather than omitted because its zero value fails closed to read-only,
// which would silently narrow a read that is supposed to mirror the owner's.
func (a dealOwnerAuthority) asOwner(ctx context.Context, owner ids.UUID) (context.Context, error) {
	member, err := authz.MemberPrincipal(ctx, a.users, owner)
	if err != nil {
		return nil, fmt.Errorf("compose: deal owner %s authority: %w", owner, err)
	}
	lang, err := a.users.ReadingLanguage(ctx, member)
	return withOwnersLanguage(ctx, member, lang, err)
}

// withOwnersLanguage binds actor, the owner or their agent, reading in the
// owner's language.
func withOwnersLanguage(ctx context.Context, actor principal.Principal, lang textlang.Lang, err error) (context.Context, error) {
	if err != nil {
		return nil, fmt.Errorf("compose: language of the owner: %w", err)
	}
	actor.Locale = string(lang)
	return principal.WithActor(ctx, actor), nil
}
