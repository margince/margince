// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// The sync-health read: the handful of overlay conditions a rep can see at a
// glance — the poller backing off, the incumbent budget degraded, mirror
// classes stale or still backfilling. It aggregates rather than enumerates
// (one concern per condition, never one per row) so a broken connector is one
// card, not a flood. The facts come from the surfaces that already own them:
// overlay_sync_state for the poller's ladder (syncbackoff.go writes it),
// SyncStatus for per-class freshness, and the OVB meter for the budget — this
// read derives nothing of its own.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/overlaybudget"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The sync-health concern vocabulary. Each names one condition; a reading
// carries at most one concern per kind.
const (
	// ConcernSyncFailing: the reconcile poller's last sweeps failed and the
	// connection is on the backoff ladder.
	ConcernSyncFailing = "sync_failing"
	// ConcernBudgetDegraded: the incumbent call budget is in its warn or shed
	// band, so live reads are throttled or declined.
	ConcernBudgetDegraded = "budget_degraded"
	// ConcernObjectsStale: at least one mirrored class holds rows the current
	// sync would not produce — stale, or carrying an undrained local write.
	ConcernObjectsStale = "objects_stale"
	// ConcernBackfillIncomplete: at least one mirrored class has not confirmed
	// its initial backfill converged.
	ConcernBackfillIncomplete = "backfill_incomplete"
)

// SyncConcern is one health condition worth a rep's glance. Kind says which;
// the remaining fields carry that kind's facts and are zero for the others.
type SyncConcern struct {
	Kind string
	// ErrorClass and Failures describe a failing sweep (the ladder's own
	// class vocabulary: auth, rate_limited, internal); NextSweepAt is when
	// the poller will try again.
	ErrorClass  string
	Failures    int
	NextSweepAt *time.Time
	// Band is the budget band a degraded budget sits in (warn or shed).
	Band string
	// Objects are the CANONICAL classes a stale/backfilling concern covers,
	// sorted for a stable card.
	Objects []string
}

// SyncHealth answers the workspace's current sync concerns, empty when
// everything is healthy. Gated exactly as SyncStatus is — every role reads
// (`overlay_connection` read), overlay mode required — so a workspace that
// never connected an incumbent answers apperrors.ErrModeNotOverlay and the
// caller renders no surface at all rather than a clear bill of health.
//
// A mirror frozen for a pending flip suppresses the staleness concern:
// staleness grows on purpose while the seal holds (ObjectSyncStatus's own
// FrozenForFlip doc), and alarming on the operator's deliberate cutover would
// train readers to ignore the lane.
func (s *Service) SyncHealth(ctx context.Context) ([]SyncConcern, error) {
	if err := auth.Require(ctx, overlayConnectionObject, principal.ActionRead); err != nil {
		return nil, err
	}
	incumbent, err := s.resolveOverlayMode(ctx)
	if err != nil {
		return nil, err
	}

	var concerns []SyncConcern
	failing, laddered, err := s.sweepFailureConcern(ctx)
	if err != nil {
		return nil, err
	}
	if laddered {
		concerns = append(concerns, failing)
	}
	if s.meter != nil {
		// Only a MEASURED snapshot can raise the concern: the meter's
		// fail-closed arms (no Redis, no config, a read error) answer shed so
		// a spender cannot overspend, but reporting that assumption here
		// would tell a rep the budget is exhausted when the truth is that
		// this role cannot account — Budget (syncstatus.go) refuses on the
		// same gap rather than fabricating a number.
		//
		// The REST band alone decides: Search consumption is metered but not
		// gated (ops.go's ConsumeSearch), so a search-window shed holds no
		// read and warning on it would cry wolf.
		snapshot := s.meter.Snapshot(ctx, incumbent)
		if snapshot.Measured && snapshot.Band != overlaybudget.BandOK {
			concerns = append(concerns, SyncConcern{Kind: ConcernBudgetDegraded, Band: snapshot.Band})
		}
	}

	objects, err := s.SyncStatus(ctx)
	if err != nil {
		return nil, err
	}
	var stale, backfilling []string
	for _, object := range objects {
		if object.State != syncStateFresh && !object.FrozenForFlip {
			stale = append(stale, object.Object)
		}
		if !object.BackfillComplete {
			backfilling = append(backfilling, object.Object)
		}
	}
	sort.Strings(stale)
	sort.Strings(backfilling)
	if len(stale) > 0 {
		concerns = append(concerns, SyncConcern{Kind: ConcernObjectsStale, Objects: stale})
	}
	// SyncStatus reports only classes that already HOLD mirror rows, so an
	// import that has not produced a first page yet — or has not started —
	// is invisible to the per-class walk above. The cursor ledger is the
	// writer's own record of convergence: any unconverged cursor, or none at
	// all in overlay mode, means the initial import is still owed, and the
	// lane must say so rather than reporting a workspace in sync that has
	// never finished importing.
	if len(backfilling) == 0 {
		owed, err := s.backfillStillOwed(ctx)
		if err != nil {
			return nil, err
		}
		if owed {
			concerns = append(concerns, SyncConcern{Kind: ConcernBackfillIncomplete})
		}
	}
	if len(backfilling) > 0 {
		concerns = append(concerns, SyncConcern{Kind: ConcernBackfillIncomplete, Objects: backfilling})
	}
	return concerns, nil
}

// backfillStillOwed answers whether the initial import has not confirmed
// convergence: an unconverged cursor row (not done, or done under a cap that
// truncated the listing), or no cursor rows at all — a backfill that never
// started is still owed, not complete.
func (s *Service) backfillStillOwed(ctx context.Context) (bool, error) {
	var unconverged, total int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FILTER (WHERE NOT done OR truncated), count(*) FROM overlay_backfill_cursor`,
		).Scan(&unconverged, &total)
	})
	if err != nil {
		return false, fmt.Errorf("overlay: reading the backfill cursors for sync health: %w", err)
	}
	return unconverged > 0 || total == 0, nil
}

// sweepFailureConcern reads the poller's backoff ladder: a row with failures
// on it is a connection that is not syncing and says when it will retry. No
// row (never swept) and a clean row both answer laddered=false.
func (s *Service) sweepFailureConcern(ctx context.Context) (concern SyncConcern, laddered bool, err error) {
	var (
		failures    int
		errorClass  *string
		nextSweepAt *time.Time
	)
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT consecutive_failures, last_error_class, next_sweep_at FROM overlay_sync_state`,
		).Scan(&failures, &errorClass, &nextSweepAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncConcern{}, false, nil
	}
	if err != nil {
		return SyncConcern{}, false, fmt.Errorf("overlay: reading the sweep ladder for sync health: %w", err)
	}
	if failures == 0 {
		return SyncConcern{}, false, nil
	}
	concern = SyncConcern{Kind: ConcernSyncFailing, Failures: failures, NextSweepAt: nextSweepAt}
	if errorClass != nil {
		concern.ErrorClass = *errorClass
	}
	return concern, true, nil
}
