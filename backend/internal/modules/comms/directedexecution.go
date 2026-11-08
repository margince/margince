// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// What authority a delivery goes out under.
//
// A message can leave for two different reasons. The engine allowed it, which
// is every message this system sent before directed sends existed. Or a named
// human read a refusal, took responsibility in writing, and decided that the
// exact message goes anyway.
//
// The delivery row says which, because the WORKER reads this row and never
// reads the per-recipient decisions beside it. A build that does not recognise
// the value parks the message rather than sending one it has no rules for.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordDirectedExecutionTx marks this delivery as going out on a named
// human's recorded decision rather than on the engine's permission.
//
// ON THE DELIVERY as well as on the per-recipient decisions, because the WORKER
// reads this row and never reads those. A build that predates directed sends
// does not know what this authority means, and the dispatcher refuses a
// delivery it cannot account for rather than sending one it has no rules for.
//
// The unique index on instruction_id is what makes this spend-once at the
// database: a second delivery reaching for the same decision is refused here
// rather than being caught by whichever caller remembered to check.
func (s *Store) RecordDirectedExecutionTx(ctx context.Context, tx pgx.Tx, deliveryID, instructionID ids.UUID) error {
	tag, err := tx.Exec(ctx, `
		UPDATE comms_outbound
		   SET execution_authority = 'instruction', instruction_id = $2
		 WHERE id = $1`, deliveryID, instructionID)
	if err != nil {
		return fmt.Errorf("comms: recording the authority this delivery goes out under: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("comms: the delivery this decision authorizes does not exist: %w",
			apperrors.ErrNotFound)
	}
	return nil
}
