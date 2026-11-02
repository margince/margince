// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var threadSplitTables = []string{
	"activity",             //nolint:goconst // schema census names the owning table
	"activity_sales_state", // A judgement stays on the source conversation.
	"capture_thread_verdict",
	"comms_outbound",
	"communication_basis",
	"signal_thread_scan",
}

func splitThreadActivityTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, from, to string) (bool, error) {
	changed, err := activities.ThreadSplitActivityTx(ctx, tx, activityID, from, to)
	if err != nil || !changed {
		return changed, err
	}
	if err := markConversationDueTx(ctx, tx, from); err != nil {
		return false, err
	}
	if err := markConversationDueTx(ctx, tx, to); err != nil {
		return false, err
	}
	return true, nil
}

func splitThreadDeliveryTx(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID, activityID ids.ActivityID, from, to string) error {
	return comms.ThreadSplitDeliveryTx(ctx, tx, deliveryID, activityID, from, to)
}

func inheritThreadSeatTx(ctx context.Context, tx pgx.Tx, from, to string, seat ids.UUID) error {
	return capture.ThreadVerdictCopySeatTx(ctx, tx, from, to, seat)
}

func inheritThreadBasisTx(ctx context.Context, tx pgx.Tx, basisID ids.UUID, from, to string) error {
	return consent.ThreadBasisCopyTx(ctx, tx, basisID, from, to)
}

func repairThreadSeatTx(ctx context.Context, tx pgx.Tx, key string, seat ids.UUID) error {
	return capture.ThreadVerdictRepairSeatTx(ctx, tx, key, seat)
}

func holdThreadSeatTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID) error {
	ctx = threadRepairContext(ctx)
	status, reason, err := capture.ThreadHoldForSeatTx(ctx, tx, id, seat)
	if err != nil || status == "" {
		return err
	}
	live, err := activities.ThreadHoldActivityTx(ctx, tx, id, reason)
	if err != nil || !live {
		return err
	}
	return capture.ThreadHoldImportTx(ctx, tx, id, seat, status, reason)
}

func invalidateThreadProofTx(ctx context.Context, tx pgx.Tx, source ids.ActivityID) (bool, error) {
	return consent.InvalidateThreadBasisTx(ctx, tx, source)
}
