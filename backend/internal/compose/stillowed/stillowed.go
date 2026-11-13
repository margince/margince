// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package stillowed answers whether the work a card or a lane proposes is
// still owed when somebody acts on it. That is a deal that still lacks a next
// step, or a promise that is still open.
package stillowed

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealsWithoutNextStepTx answers which of the given deals carry no open next
// step, by deals.OpenNextStepSQL, the predicate the overnight stager uses.
func DealsWithoutNextStepTx(
	ctx context.Context, tx pgx.Tx, candidates []ids.UUID, asOf time.Time,
) (map[ids.UUID]bool, error) {
	none := map[ids.UUID]bool{}
	if len(candidates) == 0 {
		return none, nil
	}
	for _, id := range candidates {
		none[id] = true
	}
	// Selected from the deal rather than from the activity rows, because the
	// shared predicate asks about a deal and answers over two tables.
	args := []any{candidates, asOf}
	arg := func(v any) int { args = append(args, v); return len(args) }
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return nil, err
	}
	current := storekit.CurrentTaskClause("t", asOf, identities, arg)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT d.id FROM deal d
		 WHERE d.id = ANY($1) AND %s`, deals.OpenNextStepSQL("d.id", "$2", current)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dealID ids.UUID
		if err := rows.Scan(&dealID); err != nil {
			return nil, err
		}
		delete(none, dealID)
	}
	return none, rows.Err()
}

// FollowUpTx is why an overnight follow-up for this deal is no longer owed,
// or "" while it still is. The stager asks the same two things: the deal is
// open and not archived, and it has no next step.
func FollowUpTx(ctx context.Context, tx pgx.Tx, deal ids.UUID, asOf time.Time) (string, error) {
	var open bool
	// FOR SHARE: a close or archive of the deal waits for this transaction.
	if err := tx.QueryRow(ctx, `SELECT status = 'open' AND archived_at IS NULL FROM deal WHERE id = $1 FOR SHARE`, deal).Scan(&open); err != nil {
		return "", fmt.Errorf("stillowed: reading whether the deal is open: %w", err)
	}
	if !open {
		return "This deal is no longer open, so the follow-up is not needed.", nil
	}
	none, err := DealsWithoutNextStepTx(ctx, tx, []ids.UUID{deal}, asOf)
	if err != nil {
		return "", err
	}
	if !none[deal] {
		return "This deal has a next step now, so the follow-up is not needed.", nil
	}
	return "", nil
}

// PromiseTx is whether the promise a card was staged for is still open. A card
// that names claims rests on them. One that names none rests on its recorded
// commitment decision.
func PromiseTx(
	ctx context.Context, tx pgx.Tx, claims *contacts.Store, claimIDs []ids.UUID, source ids.UUID, locator string,
) (bool, error) {
	if len(claimIDs) == 0 {
		return activities.CommitmentStillOpenTx(ctx, tx, source, locator)
	}
	return claims.ClaimsStillOpenTx(ctx, tx, claimIDs)
}
