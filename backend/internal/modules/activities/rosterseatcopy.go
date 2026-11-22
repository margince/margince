// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// oneSeatRosterRows selects the guest rows that still state a roster in the
// older single column. idx_aparticipant_one_roster_seat holds these rows alone.
const oneSeatRosterRows = `FROM activity_participant WHERE roster_seat IS NOT NULL`

// CopyRosterSeatsTx moves the one seat of up to limit older guest rows into
// their seat set, after the cursor. It returns the rows it inspected and how
// many it changed. Readers take both columns as one set, so no answer changes.
func CopyRosterSeatsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id `+oneSeatRosterRows+` AND id > $1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading guest rows that state one roster: %w", err)
	}
	inspected, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading guest rows that state one roster: %w", err)
	}
	if len(inspected) == 0 {
		return nil, 0, nil
	}
	if err := lockParticipantEvidenceParents(ctx, tx, inspected); err != nil {
		return nil, 0, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE activity_participant ap SET `+storekit.StoreRosterSeatsSQL(storekit.RosterSeatsSQL("ap"))+`
		 WHERE ap.id = ANY($1) AND ap.roster_seat IS NOT NULL`, inspected)
	if err != nil {
		return nil, 0, fmt.Errorf("activities: moving a guest row's roster into its seat set: %w", err)
	}
	return inspected, int(tag.RowsAffected()), nil
}

// CountOneSeatRosterRows counts the rows CopyRosterSeatsTx has yet to move,
// reading at most maximum+1 of them.
func CountOneSeatRosterRows(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+oneSeatRosterRows+` LIMIT $1) one_seat`, maximum+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("activities: counting guest rows that state one roster: %w", err)
	}
	return count, nil
}
