// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// A project's phase move keeps its reason on the phase history row, not on the
// audit row. The record history reads it back from there, so the history and
// the project's own read give one answer to why a move was made.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const actionAdvancePhase = "advance_phase"

// attachPhaseReasons sets Reason on each phase move of a project page. A reader
// whose role withholds the phase reads no reason, as the move's images hide it.
func attachPhaseReasons(ctx context.Context, tx pgx.Tx, entityType string, mask entityFieldMask,
	entries []RecordHistoryEntry,
) error {
	if entityType != entityTypeProject {
		return nil
	}
	if _, withheld := mask["phase"]; withheld {
		return nil
	}
	at := map[ids.UUID]int{}
	moves := []ids.UUID{}
	for i, entry := range entries {
		if entry.Action == actionAdvancePhase {
			at[entry.ID] = i
			moves = append(moves, entry.ID)
		}
	}
	if len(moves) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx,
		`SELECT audit_log_id, reason FROM project_phase_history
		  WHERE audit_log_id = ANY($1) AND reason <> ''`, moves)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var move ids.UUID
		var reason string
		if err := rows.Scan(&move, &reason); err != nil {
			return err
		}
		entries[at[move]].Reason = &reason
	}
	return rows.Err()
}
