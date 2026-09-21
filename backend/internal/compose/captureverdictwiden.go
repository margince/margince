// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Releasing the mail a `classified` mailbox held while it waited for a verdict
// on the sender.
//
// Two callers, and the split is about transaction size rather than about
// authority. The verdict's own transaction releases a bounded batch, so the
// mailbox opens the moment the sender is judged; this pass drains whatever is
// left, including every sender judged before the release existed at all.
//
// The entitlement is not passed between them. capture's predicate re-reads the
// ledger inside the statement that claims the rows, so neither caller can widen
// by asserting a verdict it did not durably record.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	// clearedSenderLiveBound is what one verdict transaction releases before
	// leaving the rest to the pass below. It shares that transaction with a
	// ledger resolution and a contact record, and the recompute takes a row lock
	// per message, so a sender with a large backlog must not drain here.
	clearedSenderLiveBound = 100
	// clearedSenderSweepBudget is one workspace's share of a reconciling pass.
	// A workspace with more held mail than this finishes on the following tick,
	// which keeps one large tenant from spending the whole fleet-wide run.
	clearedSenderSweepBudget = 2000
	// clearedSenderSweepSenders bounds how many senders one pass looks at, so
	// the scan that finds them stays a short read.
	clearedSenderSweepSenders = 50
)

// widenClearedSender releases a bounded batch of the mail this sender's verdict
// has just settled, on the verdict's own transaction.
func (e *CounterpartyVerdictEngine) widenClearedSender(ctx context.Context, tx pgx.Tx, email string) error {
	_, err := capture.WidenClearedSenderTx(ctx, tx, email, clearedSenderLiveBound, activities.RecomputeAudienceTx)
	return err
}

// WidenClearedSendersWorkspace re-opens the mail still held about senders this
// workspace has already judged to be real contacts.
//
// It is what makes the release survive a crash, a door that forgot to call the
// live hook, and — the case it was written for — every sender judged before any
// release existed, whose mail would otherwise stay limited for good because
// nothing re-asks a question the ledger considers answered.
//
// One transaction per sender rather than one for the pass: a sender's drain
// takes a row lock per message it moves, and holding those across an entire
// workspace's backlog would block readers of mail this pass is not even about.
func (e *CounterpartyVerdictEngine) WidenClearedSendersWorkspace(ctx context.Context) error {
	return e.inWorkspace(ctx, func(wsCtx context.Context, _ ids.UUID) error {
		var senders []string
		if err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
			var err error
			senders, err = capture.ClearedSendersDueTx(wsCtx, tx, clearedSenderSweepSenders)
			return err
		}); err != nil {
			return fmt.Errorf("verdict: reading the senders whose cleared mail is still held: %w", err)
		}
		budget := clearedSenderSweepBudget
		for _, email := range senders {
			if budget <= 0 {
				return nil
			}
			var moved int
			if err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
				var err error
				moved, err = capture.WidenClearedSenderAll(
					wsCtx, tx, email, budget, activities.RecomputeAudienceTx)
				return err
			}); err != nil {
				return fmt.Errorf("verdict: re-opening the mail a cleared sender's verdict settled: %w", err)
			}
			budget -= moved
		}
		return nil
	})
}
