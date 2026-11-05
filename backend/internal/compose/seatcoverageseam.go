// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seatGapLimit bounds the notice: it names colleagues so a rep knows whose
// mail to ask for, and nobody reads past a handful.
const seatGapLimit = 10

// contactSeatGaps is the one answer to "which colleagues are on this contact's
// threads without a connected mailbox", for the Network tab and the who_knows
// tool. capture cannot import compose, so the mail-provider set is passed in
// here. Callers run it after their contact read gates, in the same transaction.
func contactSeatGaps(ctx context.Context, tx pgx.Tx, contactID ids.UUID) (capture.UncapturedSeats, error) {
	return capture.SeatsWithoutMailbox(ctx, tx, contactID, MailProviders(), seatGapLimit)
}
