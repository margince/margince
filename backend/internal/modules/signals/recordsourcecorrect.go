// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// olderSignalSource selects the signals written before source_system. An
// extension's older row has none, also when its name spells a channel.
var olderSignalSource = `(NOT (` + provenance.ListedRecordSourceSQL("source") + `) OR (kind LIKE 'ext:%' AND source_system IS NULL))`

// CorrectSignalSourcesTx moves one page of signals whose source predates the
// fixed list onto its channel, and the producer it named into source_system.
// A row on the list is left out, so a replay changes nothing. Audit-only, like
// RewordScanSummaryTx: the event catalog has no signal.updated for a field.
func CorrectSignalSourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error) {
	corrected, err := storekit.CorrectRecordSources(ctx, tx, "signal",
		`SELECT id, source, signal_source_channel(source, kind) FROM signal
		  WHERE id > $1 AND `+olderSignalSource+` ORDER BY id LIMIT $2 FOR UPDATE`,
		`UPDATE signal r SET source = moved.source,
		        source_system = COALESCE(r.source_system, signal_source_producer(r.source, r.kind))
		   FROM unnest($1::uuid[], $2::text[]) AS moved(id, source) WHERE r.id = moved.id`,
		after, limit, runID)
	if err != nil {
		return nil, err
	}
	moved := make([]ids.UUID, len(corrected))
	for i, row := range corrected {
		moved[i] = row.ID
	}
	return moved, nil
}

// CountUnlistedSignalSources counts the signals CorrectSignalSourcesTx would
// still move, reading at most maximum+1.
func CountUnlistedSignalSources(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM signal WHERE `+olderSignalSource+`
		 LIMIT $1) unlisted`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("signals: counting signals with an unlisted source: %w", err)
	}
	return count, nil
}
