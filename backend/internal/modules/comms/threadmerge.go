// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ThreadMergeTx moves the deliveries of thread `from` into thread `to`, when
// capture finds the two are one conversation (compose/threadmerge.go).
//
// A delivery's thread_key names the conversation it was sent into, and readers
// that ask "what did we send in this thread" join it to the activity's key; left
// behind, a merged thread's own replies would stop counting as ours.
func ThreadMergeTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE comms_outbound SET thread_key = $2 WHERE thread_key = $1`, from, to); err != nil {
		return fmt.Errorf("comms: moving deliveries to the thread they joined: %w", err)
	}
	return nil
}
