// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// LowercaseOlderParticipantAddresses rewrites the participant rows where holds
// whose address is not lowercase, and answers how many. It changes no column:
// the table's update trigger lowercases each row and folds the rows that
// become one party. where names the row ap and takes its arguments from $1.
//
// A statement that moves rows to another party calls this first. Two spellings
// of one address moved together would otherwise meet on the identity key.
func LowercaseOlderParticipantAddresses(ctx context.Context, tx pgx.Tx, where string, args ...any) (int, error) {
	tag, err := tx.Exec(ctx, `UPDATE activity_participant ap SET display_name = ap.display_name
		 WHERE ap.address <> lower(btrim(ap.address)) AND `+where, args...)
	if err != nil {
		return 0, fmt.Errorf("store: lowercasing older participant addresses: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
