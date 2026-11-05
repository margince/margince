// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const heldMailNote = "Mailbox history: before a company mailbox was connected, it already held mail we sent them or mail we received that was also addressed to them, so the correspondence was already held and no new notice is owed."

// SettleHeldMailTx leaves queued disclosures and previously decided grounds intact.
func SettleHeldMailTx(ctx context.Context, tx pgx.Tx, acquisitions []ids.UUID, now time.Time) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	return settleAcquisitionCasesTx(ctx, tx, acquisitions, heldMailNote, now)
}
