// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Recording that a `person` verdict did NOT publish its contact.
//
// The verdict and the withholding are two facts and only the first was stored.
// Two readers ask this ledger whether a sender is a judged person and treat the
// answer as permission — the widening sweep reopens the mail a classified
// mailbox held, and the birth decision shares a future message from the same
// sender. Both matched a contact that had deliberately been kept owner-scoped,
// so the record was withheld in the moment and its correspondence published by
// the next pass anyway.
//
// The flag lives beside the verdict because that is where those readers already
// look. A decision recorded anywhere else is a decision they do not see.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MarkWithheldFromWorkspaceTx records that this sender's contact was kept to
// the mailbox owner, inside the transaction that made that decision.
//
// In the SAME transaction deliberately: a flag written afterwards can be lost
// to a crash between the two, and the state that survives is the one that says
// a withheld contact may be published.
func MarkWithheldFromWorkspaceTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE capture_pending_counterparty
		   SET withheld_from_workspace = true, updated_at = now()
		 WHERE id = $1`, id); err != nil {
		return fmt.Errorf("capture: recording that the contact was withheld: %w", err)
	}
	return nil
}
