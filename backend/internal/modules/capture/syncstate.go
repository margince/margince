// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The per-connection scheduling state machine (ADR-0063, CAP-DDL-5): a
// transient failure never kills a connection. Rate limits honor Retry-After,
// other transient errors back off exponentially, persistent failure degrades
// the connection to a daily probe — and one success heals everything. Error
// DETAIL goes to system_log; the sidecar row carries only the class.

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	// backoffBase..backoffCap bound the transient-failure retry ladder:
	// 2min·2^n capped at 4h, jittered ±20% so a fleet that failed together
	// does not retry together.
	backoffBase = 2 * time.Minute
	backoffCap  = 4 * time.Hour

	// backfillMaxConsecutiveFailures ends a run that cannot get a single page
	// through. On the ladder above, ten consecutive failures span roughly half a
	// day of retrying: a provider still refusing after that is not weather, and a
	// run making no progress for half a day is better ended visibly — the user
	// sees an error class and can restart — than left retrying where nobody looks.
	backfillMaxConsecutiveFailures = 10

	// degradeAfterFailures flips a connection to status 'error' — which means
	// "degraded, probed daily", never a tombstone: the due-scan keeps
	// selecting it at errProbeInterval and one success flips it back.
	degradeAfterFailures = 20
	errProbeInterval     = 24 * time.Hour
)

// errorClass is the CAP-DDL-5 vocabulary. The class is schedulable
// information; the underlying detail is system_log's.
type errorClass string

const (
	classRateLimited errorClass = "rate_limited"
	classUnreachable errorClass = "unreachable"
	classAuth        errorClass = "auth"
	classHistoryGone errorClass = "history_gone"
	classInternal    errorClass = "internal"
)

// classifySyncError maps a connector failure onto the shared vocabulary. Any
// error outside it is internal: our bug, not the provider's weather.
func classifySyncError(err error) errorClass {
	switch {
	case errors.Is(err, connector.ErrRateLimited):
		return classRateLimited
	case errors.Is(err, connector.ErrAuthRejected):
		return classAuth
	case errors.Is(err, connector.ErrUnreachable):
		return classUnreachable
	case errors.Is(err, connector.ErrCursorGone):
		return classHistoryGone
	default:
		return classInternal
	}
}

// backoffDelay is this connector's transient-failure ladder. The shape — double
// per failure, cap, jitter — is shared; the two bounds are this connector's own.
func backoffDelay(consecutiveFailures int) time.Duration {
	return backoff.Jittered(consecutiveFailures, backoffBase, backoffCap)
}

// recordSyncSuccess resets the ladder, paces the next sync one interval out,
// and — the auto-recovery path — flips a degraded connection back to connected
// and revives an import this connection's own trouble truncated. One success
// heals everything.
func (r *Registry) recordSyncSuccess(ctx context.Context, connectionID ids.UUID) error {
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		now := r.now()
		// The interval is a DELAY the database applies to its own clock; the
		// two last_*_at columns record when this process observed the sync and
		// stay on its clock, which is the one that observed it.
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_sync_state (connection_id, next_sync_at,
			                                consecutive_failures, last_synced_at, last_success_at,
			                                last_error_class, failing_since)
			VALUES ($1, now() + make_interval(secs => $2), 0, $3, $3, NULL, NULL)
			ON CONFLICT (connection_id) DO UPDATE SET
			  next_sync_at = now() + make_interval(secs => $2),
			  consecutive_failures = 0,
			  last_synced_at = EXCLUDED.last_synced_at,
			  last_success_at = EXCLUDED.last_success_at,
			  last_error_class = NULL,
			  -- One success ends the streak, so the next failure starts a new
			  -- one from its own instant rather than continuing this one.
			  failing_since = NULL`,
			connectionID, r.syncInterval.Seconds(), now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE capture_connection SET status = 'connected'
			WHERE id = $1 AND status = 'error' AND archived_at IS NULL`, connectionID); err != nil {
			return err
		}
		return reviveTruncatedBackfillTx(ctx, tx, connectionID)
	})
}

// reviveTruncatedBackfillTx puts a truncated import back in the queue when the
// connection it belongs to is working again.
//
// A run that ends on a terminal fault keeps its cursor, so the mailbox never has
// to be re-read from the top — but nothing used that cursor, and the run sat at
// `error` for good. The commonest cause is a credential the operator then fixes:
// rotating the connector's client app invalidates the grant mid-import, the human
// reconnects, mail flows again, and the history import stays dead with nothing
// saying so. A successful sync is the proof that the cause is gone, which is why
// the revival hangs off this write rather than a clock.
//
// It only moves the row. The reconcile pass already puts a paging job behind
// every live run and decides nothing about which — so a revived run is a live
// run, and that pass is what pages it. Nothing here reaches for a scheduler.
//
// Three guards, each of which is a way this could otherwise go wrong:
//
//   - Only a run the CREDENTIAL ended. `auth` is the one class whose remedy is a
//     human reconnecting, and a sync succeeding on this connection is the proof
//     they did — the same evidence, arriving on the same write, that flips the
//     connection back to connected. That makes the revival self-limiting: while
//     the grant is still refused no sync succeeds, so nothing revives. Every
//     other class is left alone, because no reconnection answers it and retrying
//     it on a two-minute sync interval would be a loop. The transient ladder is
//     deliberately NOT spent here: a run ended by a revoked grant needs its
//     human, not a retry, and that ladder measures something else.
//   - A run with no cursor is left alone. Reviving it would re-page the window
//     from the top and count the same messages twice, which backfillPageCursor
//     refuses for that reason. Starting a mailbox over has a cost, so it stays
//     the operator's decision.
//   - Exactly one run, and only when no other is live. uq_capture_backfill_live
//     admits one live run per connection, and this write runs inside the sync's
//     own transaction — so a second live run would not merely fail the revival,
//     it would fail the sync that triggered it and take a working mailbox down.
//
// The last guard is a predicate, and a predicate alone does not hold it. A
// human pressing Import while this sync commits passes its own NOT EXISTS at
// the same instant, and the loser of that race takes the unique violation —
// which StartBackfill answers with ErrBackfillRunning and this path cannot
// answer at all, because the statement that fails is inside somebody's
// successful sync. So both paths take the connection row first, in that order,
// and the race becomes a wait.
func reviveTruncatedBackfillTx(ctx context.Context, tx pgx.Tx, connectionID ids.UUID) error {
	if err := lockConnectionTx(ctx, tx, connectionID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE capture_backfill SET status = 'queued', completed_at = NULL`+resetInflightProgress+`
		 WHERE id = (
		         SELECT id FROM capture_backfill
		          WHERE connection_id = $1
		            AND status = 'error'
		            AND last_error_class = $2
		            AND cursor IS NOT NULL
		          ORDER BY started_at DESC
		          LIMIT 1)
		   AND NOT EXISTS (
		         SELECT 1 FROM capture_backfill
		          WHERE connection_id = $1 AND status IN ('queued','running'))`,
		connectionID, string(classAuth))
	if err != nil {
		return fmt.Errorf("capture: reviving a truncated backfill: %w", err)
	}
	return nil
}

// lockConnectionTx takes the connection row for the rest of the transaction, so
// that two paths deciding whether this connection may have a live backfill
// decide one at a time.
//
// The connection rather than the runs: there is no row to lock for a run that
// does not exist yet, which is exactly the case the insert and the revival can
// collide on. Both take it BEFORE reading capture_backfill, so the order is the
// same on both paths and neither can wait on the other.
//
// A connection that is gone locks nothing and the caller proceeds: the writes
// behind this are all keyed on the connection id, so they simply match no rows.
func lockConnectionTx(ctx context.Context, tx pgx.Tx, connectionID ids.UUID) error {
	var locked int
	err := tx.QueryRow(ctx,
		`SELECT 1 FROM capture_connection WHERE id = $1 FOR UPDATE`, connectionID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: locking the connection: %w", err)
	}
	return nil
}

// recordSyncFailure classifies, schedules the retry, and degrades — never
// tombstones. Auth parks the connection as reauth_required until its human
// reconnects (the OAuth callback resets both rows).
//
// It carries no generation predicate, and does not need one: every
// capture_connection write here is guarded by the status it moves FROM
// ('connected'/'error'), and that guard is the fence — a disconnected or
// reauth-parked row matches nothing, so a cycle that started before its human
// acted can never drag the row back to a healthier status. What it records is
// the connection's own health, which outlives any one grant: the daily probe of
// a degraded connection has to be able to write its verdict.
func (r *Registry) recordSyncFailure(ctx context.Context, connectionID ids.UUID, syncErr error) error {
	class := classifySyncError(syncErr)
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		now := r.now()

		var failures int
		if err := tx.QueryRow(ctx, `
			INSERT INTO capture_sync_state (connection_id, next_sync_at,
			                                consecutive_failures, last_synced_at,
			                                last_error_class, failing_since)
			VALUES ($1, now() + make_interval(secs => $2), 1, $3, $4, $3)
			ON CONFLICT (connection_id) DO UPDATE SET
			  consecutive_failures = capture_sync_state.consecutive_failures + 1,
			  last_synced_at = EXCLUDED.last_synced_at,
			  last_error_class = EXCLUDED.last_error_class,
			  -- SET ONCE PER STREAK, and this COALESCE is the whole point of the
			  -- column. last_synced_at beside it moves on every tick, so it dates
			  -- the newest attempt; a duration read off it would say "failing for
			  -- two minutes" through an outage of any length. The streak's own
			  -- start does not move until a success clears it.
			  failing_since = COALESCE(capture_sync_state.failing_since, EXCLUDED.failing_since)
			RETURNING consecutive_failures`,
			connectionID, backoffDelay(0).Seconds(), now, string(class)).Scan(&failures); err != nil {
			return err
		}

		// A DELAY from here on, never a deadline: the write below hands it to
		// the database, which adds it to the clock the due-scan reads.
		delay := backoffDelay(failures)
		switch class {
		case classAuth:
			// The connection needs its human, not a retry: park it. The
			// due-scan only selects connected/error, so no next_sync_at
			// gymnastics are needed.
			if _, err := tx.Exec(ctx, `
				UPDATE capture_connection SET status = 'reauth_required'
				WHERE id = $1 AND status IN ('connected','error') AND archived_at IS NULL`, connectionID); err != nil {
				return err
			}
		case classRateLimited:
			var rl *connector.RateLimitedError
			if errors.As(syncErr, &rl) && rl.RetryAfter > delay {
				delay = rl.RetryAfter
			}
		default:
		}

		if failures >= degradeAfterFailures {
			// Degraded, probed daily — never a tombstone. One success in the
			// daily probe flips the status back (recordSyncSuccess).
			delay = errProbeInterval
			if _, err := tx.Exec(ctx, `
				UPDATE capture_connection SET status = 'error'
				WHERE id = $1 AND status = 'connected' AND archived_at IS NULL`, connectionID); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `
			UPDATE capture_sync_state SET next_sync_at = now() + make_interval(secs => $2)
			WHERE connection_id = $1`,
			connectionID, delay.Seconds()); err != nil {
			return err
		}

		// The class is on the row; the detail belongs to the operational
		// ledger (0078's rationale, kept).
		if _, err := storekit.LogSystem(ctx, tx, "capture_sync_error", map[string]any{
			detailConnectionID: connectionID.String(),
			"class":            string(class),
			"failures":         failures,
			"detail":           syncErr.Error(),
		}); err != nil {
			return err
		}
		return nil
	})
}
