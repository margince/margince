// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// Read sharing never grants write authority. Contract writes use the same
// effective anchor as reads and require write access to that anchor.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	// nil columns: this reads only to PROVE visibility and throws the row
	// away, so fetching custom values here would be work nobody reads.
	if _, err := readContract(ctx, tx, id, asOf, nil); err != nil {
		return crmcontracts.Contract{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, contractTable, id.UUID, storekit.IncludeArchived); err != nil {
		return crmcontracts.Contract{}, err
	}
	existing, err := readContract(ctx, tx, id, asOf, nil)
	if err != nil {
		return crmcontracts.Contract{}, err
	}
	if err := ensureAnchorWritable(ctx, tx, existing); err != nil {
		return crmcontracts.Contract{}, err
	}
	return existing, nil
}

// ensureAnchorWritable requires write access to the effective anchor;
// a read share alone never grants it.
func ensureAnchorWritable(ctx context.Context, tx pgx.Tx, contract crmcontracts.Contract) error {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	clause, err := auth.ContractVisibleClause(ctx, "c", arg)
	if err != nil {
		return err
	}
	kindSQL, idSQL := auth.ContractAnchorSQL("c", "d")
	query := fmt.Sprintf(`SELECT %s, %s FROM contract c LEFT JOIN deal d ON d.id = c.deal_id
  WHERE c.id = $%d AND %s`, kindSQL, idSQL, arg(contract.Id), clause)
	var anchor string
	var anchorID ids.UUID
	if err := tx.QueryRow(ctx, query, args...).Scan(&anchor, &anchorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	}
	if anchor == companyTable {
		if _, err := anchorOf(contract); err != nil {
			return err
		}
	}
	return auth.EnsureWritable(ctx, tx, anchor, anchorID)
}
