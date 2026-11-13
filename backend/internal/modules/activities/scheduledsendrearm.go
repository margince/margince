// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Finding a scheduled message whose alarm is gone, and giving it a new one.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// zoneRuleShift bounds how far a chosen local time's instant can move when its
// zone's rules change: the UTC offsets in use span UTC-12 to UTC+14.
const zoneRuleShift = 26 * time.Hour

// overdueScheduledSendsSQL reads idx_scheduled_send_due in cached-instant order
// up to zoneRuleShift past the cutoff, and resolves the chosen local time only
// for the rows in that range. A row re-armed after the cutoff still has that
// alarm in flight, so the batch passes it by and reaches the rows behind it.
const overdueScheduledSendsSQL = `
	SELECT id FROM scheduled_send
	 WHERE status = 'scheduled' AND scheduled_at < @horizon AND ` + scheduleInstantSQL + ` < @before
	   AND (rearmed_at IS NULL OR rearmed_at < @before)
	 ORDER BY scheduled_at ASC
	 LIMIT @limit`

func overdueScheduledSendsArgs(before time.Time, limit int) pgx.NamedArgs {
	return pgx.NamedArgs{"before": before, "horizon": before.Add(zoneRuleShift), "limit": limit}
}

// OverdueScheduledSends returns waiting messages whose moment has passed. A
// message reaches that state when the job that should have fired it is gone.
// compose/scheduledsendrecovery.go, the only caller, re-arms them.
//
// `olderThan` keeps it off messages still in flight. A row due seconds ago more
// likely has a job about to run, and re-arming it would race that job. A row
// re-armed less than `olderThan` ago is left out for the same reason.
//
// There is no workspace predicate: an installation holds one workspace. A
// chosen local time resolves here too, so a zone rule change that makes a send
// due earlier does not leave it waiting.
func (s *Store) OverdueScheduledSends(ctx context.Context, olderThan time.Duration, limit int) ([]ids.UUID, error) {
	var out []ids.UUID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, overdueScheduledSendsSQL, overdueScheduledSendsArgs(s.now().Add(-olderThan), limit))
		if err != nil {
			return fmt.Errorf("scheduled send: finding messages nothing will wake: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("scheduled send: reading an overdue message: %w", err)
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	return out, err
}

// RearmScheduledSend enqueues a fresh alarm for a message whose own alarm is
// gone, in one transaction with the claim that proves it is still waiting.
//
// It leaves the message's moment and state alone. The ordinary fire path then
// makes the ordinary decision, so the sweep is never a second send path.
//
// It stamps rearmed_at, which only OverdueScheduledSends reads. The stamp is the
// sweep's cursor, like a retry's next_attempt_at. Nothing about the message
// changed, so it writes no audit row and no event.
//
// The claim lock serializes it against a real timer firing at the same instant.
func (s *Store) RearmScheduledSend(ctx context.Context, id ids.UUID, timer ScheduleTimer) error {
	if timer == nil {
		return errNoScheduleTimer
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		claimed, found, err := s.claimForFire(ctx, tx, id)
		if err != nil {
			return err
		}
		if !found {
			// Sent, cancelled or held while this pass was reading. Nothing to
			// re-arm, and nothing wrong.
			return nil
		}
		tag, err := tx.Exec(ctx, `UPDATE scheduled_send SET rearmed_at = $2 WHERE id = $1 AND version = $3`,
			id, s.now(), claimed.RowVersion)
		if err != nil {
			return fmt.Errorf("scheduled send: stamping the re-arm: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// The claim holds the row locked, so only a caller that skipped
			// the claim can reach here.
			return fmt.Errorf("scheduled send: %s moved while it was claimed for re-arming", id)
		}
		// Armed for the moment the row holds, which has passed, so it runs at
		// once. The timer coalesces it with an alarm for that moment still live.
		return timer.ScheduleTx(ctx, tx, id, claimed.ScheduledAt)
	})
}
