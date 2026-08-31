// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// The sync-health read: the handful of overlay conditions a rep can see at a
// glance — the poller backing off, the incumbent budget degraded, mirror
// classes stale or still backfilling, records overwritten from the other side.
// It aggregates rather than enumerates (one concern per condition, never one
// per row) so a broken connector is one card, not a flood. The facts come from
// the surfaces that already own them: overlay_sync_state for the poller's
// ladder (syncbackoff.go writes it), SyncStatus for per-class freshness, the
// OVB meter for the budget, and the system_log ledger the reconcile sweep
// writes for an overwrite — this read derives nothing of its own.

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
	// ConcernRecordsOverwritten: an incumbent-driven write overwrote a mirror
	// row somebody had edited here. The one concern about work already lost
	// rather than work not happening, which is why it is on the lane at all:
	// the overwrite is committed and the reader's only move is to go and look.
	ConcernRecordsOverwritten = "records_overwritten"
)

// overwriteWindow bounds how far back the overwrite concern looks.
//
// Every other concern on this lane is a CONDITION that is still true — a
// ladder still climbing, a budget still shed — and clears itself when it
// stops. An overwrite is a past act that never stops being true, so without a
// window the lane would carry the first conflict a workspace ever had for the
// rest of its life. A day is the Worklist's own rhythm: it is read each
// morning against what happened since the last one, and an overwrite older
// than that is the record's history, which the record's page holds.
const overwriteWindow = 24 * time.Hour

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

	overwrite, overwritten, err := s.overwrittenConcern(ctx)
	if err != nil {
		return nil, err
	}
	if overwritten {
		concerns = append(concerns, overwrite)
	}

	objectConcerns, err := s.objectConcerns(ctx)
	if err != nil {
		return nil, err
	}
	return append(concerns, objectConcerns...), nil
}

// overwrittenConcern names the mirror classes an incumbent-driven write
// overwrote inside the window, aggregated to one concern the way every other
// one here is: a sweep that overwrote four hundred rows is a card saying which
// KINDS of record to go and check, not four hundred cards.
//
// It reads the system_log ledger the reconcile sweep writes beside each
// mirror.conflict event, not the event stream: the outbox is drained and gone,
// and the log is append-only, so the log is the only durable record that the
// overwrite happened. Rows carrying no object_class are skipped rather than
// reported as an unnamed class — a card that says "something was overwritten"
// sends the reader nowhere.
func (s *Service) overwrittenConcern(ctx context.Context) (concern SyncConcern, overwritten bool, err error) {
	var classes []string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT detail->>'object_class'
			FROM system_log
			WHERE action = $1
			  AND occurred_at > now() - $2::interval
			  AND detail->>'object_class' IS NOT NULL
			ORDER BY 1`, mirrorConflictAction, overwriteWindow)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var class string
			if err := rows.Scan(&class); err != nil {
				return err
			}
			classes = append(classes, class)
		}
		return rows.Err()
	})
	if err != nil {
		return SyncConcern{}, false, fmt.Errorf("overlay: reading the overwrite ledger for sync health: %w", err)
	}
	if len(classes) == 0 {
		return SyncConcern{}, false, nil
	}
	return SyncConcern{Kind: ConcernRecordsOverwritten, Objects: classes}, true, nil
}

// objectConcerns derives the per-class concerns: stale mirror classes, and an
// initial import that has not confirmed convergence.
func (s *Service) objectConcerns(ctx context.Context) ([]SyncConcern, error) {
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
	var concerns []SyncConcern
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
		return concerns, nil
	}
	return append(concerns, SyncConcern{Kind: ConcernBackfillIncomplete, Objects: backfilling}), nil
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
