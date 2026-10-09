// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Putting the name we learned onto the page, for contacts whose split columns
// were filled before the display followed.
//
// A contact minted from a calendar invitation is named the way the organizer had
// them saved: "Bw" for Björn Welter. A signature later teaches the record the
// real name, which lands in first_name and last_name. The fill now moves the
// display with them; this pass repairs the rows the old fill left behind. It
// replaces only a name capture guessed (contacts.StaleDisplayNamesTx).
//
// It is separate from the attendee-name recovery beside it, which drains on
// activity_participant.display_name IS NULL. These rows' attendee names are
// already recovered; it is the contact's display that stayed behind.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// repairStaleDisplayNamesBatch puts the learned name on the page for up to limit
// contacts whose display still shows what a machine first guessed.
func repairStaleDisplayNamesBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	// storekit.EmitEvent REFUSES to publish without a correlation id, so a pass
	// that omitted one would repair nothing at all: every contact would fail on
	// their own event, the batch would roll back, and the backlog would sit there
	// looking as though the job had simply not run yet.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	var repaired int
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		stale, err := contacts.StaleDisplayNamesTx(ctx, tx, limit)
		if err != nil {
			return err
		}
		for _, contactID := range stale {
			// The writer asks the selector's question again under the row lock.
			// A name somebody typed since the select is still theirs.
			moved, err := contacts.RefreshDisplayNameTx(ctx, tx, contactID)
			if err != nil {
				return err
			}
			if moved {
				repaired++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if repaired > 0 {
		log.InfoContext(ctx, "display name repair: contacts now show the name we learned",
			"contacts", repaired)
	}
	return repaired, nil
}
