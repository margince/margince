// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// importer is who one capture_import row names: a seat, or a shared mailbox's
// connection. The CHECK capture_import_one_importer holds that a row names
// one of the two.
type importer struct {
	id ids.UUID
	// column is the capture_import column holding id.
	column string
	// conflict is the unique index one importer's row per message lands on.
	conflict string
	// connection matches the delivering capture_connection as alias c.
	connection string
}

// importerOf answers the importer a capture writes its import row as.
func importerOf(ctx context.Context, seat ids.UUID) (importer, bool) {
	if !seat.IsZero() {
		return seatImporter(seat), true
	}
	if shared, ok := sharedMailboxOf(ctx); ok {
		return mailboxImporter(shared), true
	}
	return importer{}, false
}

func seatImporter(seat ids.UUID) importer {
	return importer{
		id: seat, column: "user_id", conflict: "(activity_id, user_id)",
		connection: "c.user_id = @importer AND c.provider = @delivering_provider",
	}
}

func mailboxImporter(connection ids.UUID) importer {
	return importer{
		id: connection, column: "connection_id", conflict: "(connection_id, activity_id) WHERE connection_id IS NOT NULL",
		connection: "c.id = @importer AND c.user_id IS NULL",
	}
}

// importerHoldsActivityTx answers whether the row is already this importer's:
// the seat's standing, or the shared mailbox's own earlier import row.
func importerHoldsActivityTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, shared ids.UUID, isShared bool) (bool, error) {
	if !isShared {
		return SeatHoldsActivityTx(ctx, tx, id)
	}
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM capture_import
	  WHERE activity_id = $1 AND connection_id = $2 AND user_id IS NULL)`, id, shared).Scan(&held); err != nil {
		return false, fmt.Errorf("capture: reading whether the shared mailbox imported %s: %w", id, err)
	}
	return held, nil
}
