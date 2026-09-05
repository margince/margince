// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// Which findings a run observed.
//
// The run row says how MUCH was checked and the exception row says what is open
// NOW. Neither answers "which run last confirmed this exception", which is the
// first question a manager doubting a finding asks: is this still true, or is it
// a leftover from a night before the deal moved?
//
// Recorded once per run rather than once per finding. The scan already collects
// every logical key it minted (scan.go, `seen`), and those keys are the
// exception's stable identity — so one statement resolves them all, and a rule
// that fired twice on one key records one observation rather than colliding.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordRunFindings records that this run observed the exceptions named by
// `seen`, at the run's own as-of.
//
// The keys are LOGICAL, not row ids: the scan works in logical keys because
// that is what makes a finding the same finding across nights, and resolving
// them here keeps UpsertException's signature about upserting rather than about
// bookkeeping.
//
// EVERY exception the key names, whatever its status. A finding somebody
// resolved is still a finding this run OBSERVED, and dropping it here would make
// "which run last confirmed this" answer nothing for exactly the exceptions a
// manager is most likely to question.
//
// A key naming no exception at all is skipped rather than refused: the same
// transaction just upserted every one of them, so a miss can only mean the row
// went in between, and failing the night's run over bookkeeping would lose the
// findings themselves.
func (s *Store) RecordRunFindings(
	ctx context.Context, tx pgx.Tx, runID ids.UUID, seen []string,
) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	if len(seen) == 0 {
		return nil
	}
	// observed_at comes from the RUN, not from an argument. A caller-supplied
	// instant is a second copy of a fact the run already holds, and the two can
	// drift: a membership stamped later than a real later run would reorder
	// "which run last confirmed this" and answer with the wrong night.
	//
	// ON CONFLICT DO NOTHING over the (run, exception) key: one run observing
	// one finding is one row, however many rules or subjects produced it.
	if _, err := tx.Exec(ctx, `
		INSERT INTO assurance_run_finding (run_id, exception_id, observed_at)
		SELECT r.id, e.id, r.as_of
		  FROM assurance_run r, assurance_exception e
		 WHERE r.id = $1 AND e.logical_key = ANY($2)
		ON CONFLICT (run_id, exception_id) DO NOTHING`,
		runID, seen); err != nil {
		return fmt.Errorf("assurance: recording which findings this run saw: %w", err)
	}
	return nil
}
