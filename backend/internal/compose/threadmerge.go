// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One conversation, one thread key — everywhere a row carries one.
//
// Capture finds that two thread keys are one conversation when an email's reply
// links reach both (capture/threadjoin.go). Six tables key a row on a thread,
// owned by four modules and this package, so the merge is composed here: each
// owner moves its own rows, in one transaction with the capture that found the
// link.
//
// Held by: TestEveryThreadKeyedTableIsMerged
// (compose/threadmerge_integration_test.go), which reads the live schema and
// fails when a table gains a thread_key column this merge does not cover.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
)

// threadMergedTables names every table mergeThreadsTx rewrites, for the
// schema check above to compare against.
var threadMergedTables = []string{
	"activity", //nolint:goconst // a table name here, not the record type recordTypeActivity names
	"activity_sales_state",
	"capture_thread_verdict",
	"comms_outbound",
	"communication_basis",
	"signal_thread_scan",
}

// mergeThreadsTx moves thread `from` into thread `to`.
//
// Nothing here changes who may read a message that is already stored: a
// message's audience derives from its own capture_import rows, which carry the
// verdict it was imported under, and no thread key. What moves is what the NEXT
// message of the conversation inherits, and capture.ThreadVerdictMergeTx keeps
// that at least as closed as either half was.
func mergeThreadsTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	if err := activities.ThreadMergeTx(ctx, tx, from, to); err != nil {
		return err
	}
	if err := capture.ThreadVerdictMergeTx(ctx, tx, from, to); err != nil {
		return err
	}
	if err := comms.ThreadMergeTx(ctx, tx, from, to); err != nil {
		return err
	}
	if err := consent.ThreadMergeTx(ctx, tx, from, to); err != nil {
		return err
	}
	// The signal scan's bookkeeping for both halves. It records how far a
	// thread was read; the merged thread is a conversation neither scan read
	// whole, so it is read again from the start.
	if _, err := tx.Exec(ctx, `
		DELETE FROM signal_thread_scan WHERE thread_key IN ($1, $2)`, from, to); err != nil {
		return fmt.Errorf("compose: resetting the signal scan of merged threads: %w", err)
	}
	return nil
}

// threadJoiner wires capture's thread join to the modules that own its tables.
func threadJoiner() capture.ThreadJoiner {
	return capture.ThreadJoiner{
		RecordReferences: activities.RecordMailReferencesTx,
		Neighbours:       activities.MailNeighboursTx,
		Earliest:         activities.EarliestThreadKeyTx,
		Merge:            mergeThreadsTx,
		Move:             activities.MoveMessageToThreadTx,
	}
}
