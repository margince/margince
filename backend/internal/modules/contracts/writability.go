// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// Contract write authority, inherited from the same anchor visibility.go
// derives sight from — and inherited in BOTH directions, which is the whole
// point of this file.
//
// A contract carries no owner_id, so every gate it has is its anchor's. The
// visibility clause answers "may this caller SEE the agreement", and until this
// file existed it was also the only thing standing in front of a patch, an
// archive, a status change, a cancellation and a renewal. A manual grant widens
// visibility at either access level (that is what makes a `read` share useful),
// so a colleague handed read on a deal could rewrite, cancel and archive every
// agreement hanging off it (#1373).
//
// The anchor rule is visibility.go's, restated once here rather than re-derived:
// a contract WITH a deal is judged by that deal, and one without is judged by
// its company. Widening a deal-anchored contract to its company would hand
// a caller agreements attached to deals they cannot see; narrowing it to both
// would refuse a legitimate editor who holds only the deal.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// writableContract is readContract for a path that CHANGES the agreement: the
// caller must be able to see it, and their authority over its anchor must be
// write-level.
//
// The read comes first and keeps its 404, so a caller who cannot see the
// contract still cannot learn it exists by trying to change it; only a caller
// who has been shown the row is answered ErrPermissionDenied.
//
// It then LOCKS and reads AGAIN, and the second read is the one the write uses.
// Every caller here decides something from what it read — a renewal refuses a
// predecessor that is already superseded, a transition validates against the
// status it saw, and every patch takes its audit "before" image from it — while
// the row lock arrived only at ApplyGuarded, after all of that. So two
// concurrent renewals both read `active`, both minted a successor and then
// serialised on the late lock: the second overwrote superseded_by_id, both
// successors committed, and the chain this module calls single-headed had two
// heads with one orphaned.
//
// The lock includes ARCHIVED rows on purpose. readContract does not filter the
// contract's own archived_at — its clause is about the ANCHOR's — so LiveOnly
// here would refuse a write this module admits today, which is a different
// change from the ordering one.
//
// The visibility read stays FIRST, and what it buys is not the 404 — the read
// UNDER the lock answers that either way, since LockRow carries no visibility
// clause and the row is refused the moment it is read. What it buys is that an
// authenticated caller who cannot see a contract does not get to take a lock on
// it: without it, a caller outside the row scope would queue every writer of an
// agreement they may not even know exists, for the length of their own
// transaction.
func writableContract(ctx context.Context, tx pgx.Tx, id ids.ContractID, asOf time.Time) (crmcontracts.Contract, error) {
	if _, err := readContract(ctx, tx, id, asOf); err != nil {
		return crmcontracts.Contract{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, contractTable, id.UUID, storekit.IncludeArchived); err != nil {
		return crmcontracts.Contract{}, err
	}
	existing, err := readContract(ctx, tx, id, asOf)
	if err != nil {
		return crmcontracts.Contract{}, err
	}
	if err := ensureAnchorWritable(ctx, tx, existing); err != nil {
		return crmcontracts.Contract{}, err
	}
	return existing, nil
}

// ensureAnchorWritable applies the write-authority probe to whichever record
// the contract inherits from.
//
// It asks for authority and NOT for liveness, and that is deliberate rather
// than the omission it looks like next to its siblings elsewhere. The liveness
// is already spent: writableContract reads through readContract, which composes
// VisibleClause, whose two anchor arms both carry archived_at IS NULL. A caller
// who cannot read a contract on an archived anchor cannot reach this probe at
// all, so repeating the filter here would guard nothing a human can do.
//
// The write therefore AGREES with the read rather than being stricter than it.
// Making it stricter would separate the two for a caller unbounded on BOTH
// anchors, which today is the system principal, and one question answered by
// two gates is how the next reader ends up unable to tell which is
// authoritative.
func ensureAnchorWritable(ctx context.Context, tx pgx.Tx, contract crmcontracts.Contract) error {
	if contract.DealId != nil {
		return auth.EnsureWritable(ctx, tx, dealTable, ids.UUID(*contract.DealId))
	}
	anchor, err := anchorOf(contract)
	if err != nil {
		return err
	}
	return auth.EnsureWritable(ctx, tx, companyTable, anchor)
}
