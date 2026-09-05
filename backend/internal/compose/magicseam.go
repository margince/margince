// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Wiring the machinery's receipt.
//
// The one edge that needs explaining is the brief: the window this surface
// reports over defaults to when the night last READ the records, which is what
// the reader has already seen. That instant lives in compose/briefs, a sibling
// package, so it arrives through a seam rather than an import.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// newMagicService assembles the receipt's read.
func newMagicService(pool *pgxpool.Pool, now func() time.Time) *magic.Service {
	db := InstallationDB(pool)
	return magic.NewService(pool, magicBriefCutoff{
		engine: briefs.NewBriefEngine(pool, nil),
		now:    now,
	}, now).
		WithTroubledRuns(automation.NewAutomationStore(db))
}

// magicBriefCutoff answers when the acting rep's night last read the records.
//
// The run's as_of, not its generated_at. A run written at 06:42 over records
// read at 06:00 has a 42-minute window in which the machinery kept working, and
// reporting from the write time would hide exactly the overnight work this
// surface exists to show.
type magicBriefCutoff struct {
	engine *briefs.BriefEngine
	now    func() time.Time
}

// CutoffFor answers the cutoff and whether a run exists at all.
//
// No run is not a failure: the reader simply has no brief to date the window
// from, and the service falls back to a day. The refusal is reported as
// not-found rather than as an error for the same reason — a rep who has never
// had a brief is an ordinary state, not a broken read.
func (m magicBriefCutoff) CutoffFor(ctx context.Context) (time.Time, bool, error) {
	run, err := m.engine.LatestRun(ctx, m.now())
	if errors.Is(err, apperrors.ErrNotFound) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return run.AsOf, true, nil
}
