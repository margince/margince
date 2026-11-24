// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The nightly close-date sweep used to write a halved probability on a quiet
// deal. The quiet effect now moves the win chance instead. That write stands for
// nothing and would pin the deal at the halved figure. This clears it and
// nothing else. A probability a colleague or a verdict set since is another
// write and stays.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// winProbabilityField is the wire field a clear names to put the deal back on
// its stage.
const winProbabilityField = "win_probability"

// halvedProbabilityCandidatesSQL is the open deals whose own probability
// Margince set. Columns: id.
const halvedProbabilityCandidatesSQL = `
	FROM deal d
	WHERE d.win_probability_set_by = 'margince' AND d.win_probability_override IS NOT NULL
	  AND d.status = 'open' AND d.archived_at IS NULL`

// lastProbabilityWriteSQL reads the newest audit write that changed a deal's own
// probability. Columns: written by the close-date downgrade, the value it wrote.
const lastProbabilityWriteSQL = `
	SELECT a.evidence->>'` + CloseDateCorrectionKind + `' = 'downgrade_and_review',
	       (a.after->>'` + winProbabilityOverrideColumn + `')::int
	  FROM audit_log a
	 WHERE a.entity_type = 'deal' AND a.entity_id = $1
	   AND a.after->>'` + winProbabilityOverrideColumn + `' IS DISTINCT FROM a.before->>'` + winProbabilityOverrideColumn + `'
	 ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1`

// RetireHalvedProbabilitiesTx clears, for up to limit deals after the cursor,
// the probability the close-date downgrade wrote. It clears only when the deal
// still holds that value and nothing has changed it since. It answers every deal
// it inspected and how many it cleared. The write is the ordinary deal update,
// so it keeps its audit row and event.
func (s *Store) RetireHalvedProbabilitiesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT d.id, d.win_probability_override `+halvedProbabilityCandidatesSQL+`
		AND d.id > $1 ORDER BY d.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("deals: reading the halved probabilities: %w", err)
	}
	type candidate struct {
		id    ids.UUID
		value int
	}
	page, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (candidate, error) {
		var c candidate
		err := row.Scan(&c.id, &c.value)
		return c, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("deals: reading the halved probabilities: %w", err)
	}
	inspected, cleared := make([]ids.UUID, 0, len(page)), 0
	for _, c := range page {
		inspected = append(inspected, c.id)
		written, err := s.retireIfStillHalved(ctx, tx, c.id, c.value)
		if err != nil {
			return nil, 0, err
		}
		if written {
			cleared++
		}
	}
	return inspected, cleared, nil
}

func (s *Store) retireIfStillHalved(ctx context.Context, tx pgx.Tx, id ids.UUID, held int) (bool, error) {
	var bySweep *bool
	var wrote *int
	if err := tx.QueryRow(ctx, lastProbabilityWriteSQL, id).Scan(&bySweep, &wrote); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("deals: reading how a probability was written: %w", err)
	}
	if bySweep == nil || !*bySweep || wrote == nil || *wrote != held {
		return false, nil
	}
	if _, err := s.UpdateDealTx(ctx, tx, ids.From[ids.DealKind](id), UpdateDealInput{Clear: []string{winProbabilityField}}, CustomColumns{}); err != nil {
		return false, fmt.Errorf("deals: clearing a halved probability: %w", err)
	}
	return true, nil
}

// CountHalvedProbabilityCandidates counts the deals the task inspects, to the
// bound it is given.
func CountHalvedProbabilityCandidates(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT d.id `+halvedProbabilityCandidatesSQL+` LIMIT $1) candidates`, maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}
