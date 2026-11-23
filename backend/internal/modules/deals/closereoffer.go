// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A close for silence that rested on incomplete mail comes back to its owner.
// The close stays what it was. The correction row gains an instant that puts
// it on Home again, where the one existing Undo reopens the deal.

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

// SilentClose is one close for silence that Margince made and its owner has
// neither undone nor accepted, and that is still the deal's newest move.
type SilentClose struct {
	CorrectionID ids.UUID
	Deal         ids.DealID
	Owner        ids.UUID
	ClosedAt     time.Time
}

// SilentClosesTx lists up to limit such closes made since an instant, after the
// correction id given, in id order. A close already offered back is left out.
func (s *Store) SilentClosesTx(ctx context.Context, tx pgx.Tx, since time.Time, after ids.UUID, limit int) ([]SilentClose, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := storekit.SQLf(`
		SELECT c.id, c.deal_id, d.owner_id, c.applied_at
		  FROM deal_correction c JOIN deal d ON d.id = c.deal_id
		  JOIN audit_log a ON a.id = c.audit_log_id AND a.entity_type = 'deal' AND a.entity_id = c.deal_id
		 WHERE c.correction = $%d AND c.reversed_at IS NULL AND c.accepted_at IS NULL AND c.reoffered_at IS NULL
		   AND c.applied_at >= $%d AND c.id > $%d
		   AND d.lost_reason_code = $%d AND d.archived_at IS NULL AND d.owner_id IS NOT NULL
		   AND %s
		 ORDER BY c.id LIMIT $%d`,
		arg(CloseLostCorrection), arg(since), arg(after), arg(LossReasonWentSilent),
		correctionStageStillNewestSQL(arg(stageCorrectionKinds)), arg(limit))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("deals: listing the closes for silence: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SilentClose, error) {
		var c SilentClose
		err := row.Scan(&c.CorrectionID, &c.Deal, &c.Owner, &c.ClosedAt)
		return c, err
	})
}

// ReofferClose puts one close for silence on its owner's Home again, once.
// It offers nothing when the owner undid or accepted the close, the deal moved
// since, or the close was offered back already. It answers whether it offered.
// Record writes the reason on the deal's timeline in the same transaction.
func (s *Store) ReofferClose(
	ctx context.Context, closed SilentClose, record func(ctx context.Context, tx pgx.Tx) error,
) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	offered := false
	err := storekit.RetryLockCycles(ctx, func() error {
		return s.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			offered, err = reofferCloseTx(ctx, tx, closed, record)
			return err
		})
	})
	return offered, err
}

func reofferCloseTx(ctx context.Context, tx pgx.Tx, closed SilentClose, record func(ctx context.Context, tx pgx.Tx) error) (bool, error) {
	if _, err := storekit.LockRow(ctx, tx, dealTable, closed.Deal.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var held bool
	err := tx.QueryRow(ctx, storekit.SQLf(`
		SELECT EXISTS (SELECT 1 FROM deal_correction c JOIN deal d ON d.id = c.deal_id
		 WHERE c.id = $1 AND c.deal_id = $2 AND c.correction = $3 AND c.reversed_at IS NULL
		   AND c.accepted_at IS NULL AND c.reoffered_at IS NULL AND d.lost_reason_code = $4 AND %s)`,
		correctionStageStillNewestSQL(5)), closed.CorrectionID, closed.Deal, CloseLostCorrection, LossReasonWentSilent, stageCorrectionKinds).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("deals: re-reading the close to offer back: %w", err)
	}
	if !held {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE deal_correction SET reoffered_at = now(), updated_at = now(), version = version + 1
		 WHERE id = $1`, closed.CorrectionID); err != nil {
		return false, fmt.Errorf("deals: offering the close back: %w", err)
	}
	change := func(offered bool) map[string]any {
		return map[string]any{"applied_change": map[string]any{"id": closed.CorrectionID.String(), "offered_again": offered}}
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "deal", closed.Deal.UUID, change(false), change(true))
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, closed.Deal.UUID, crmcontracts.PublicEventDealUpdated{ChangedFields: change(true)}); err != nil {
		return false, err
	}
	return true, record(ctx, tx)
}

// correctionListedSQL is the instant a correction c counts from on Home: its
// own time, or the time it was offered back, whichever is later. The list and
// its fingerprint read it alike.
const correctionListedSQL = "greatest(c.applied_at, c.reoffered_at)"
