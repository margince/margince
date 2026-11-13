// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// cloneOverdue copies one written message count times, each copy a second
// later than the one before, and returns every copy oldest first.
func (p *preflightEnv) cloneOverdue(t *testing.T, source ids.UUID, count int) []ids.UUID {
	t.Helper()
	var copies []ids.UUID
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			INSERT INTO scheduled_send
			SELECT (jsonb_populate_record(s, jsonb_build_object(
			         'id', uuidv7(),
			         'scheduled_at', s.scheduled_at + n * interval '1 second'))).*
			  FROM scheduled_send s, generate_series(1, $2::int) AS n
			 WHERE s.id = $1
			RETURNING id`, source, count)
		if err != nil {
			return err
		}
		copies, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		t.Fatalf("cloning the overdue message: %v", err)
	}
	return copies
}

// forgetRearm clears a message's re-arm stamp, the state its row reaches once
// the recovery grace has passed while the alarm it got is still queued.
func (p *preflightEnv) forgetRearm(t *testing.T, id ids.UUID) {
	t.Helper()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE scheduled_send SET rearmed_at = NULL WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("clearing the re-arm stamp: %v", err)
	}
}

// overdueMessage schedules one message through the API and moves its moment
// well past the recovery grace, so the pass treats it as stranded. Wall time,
// because the API and the recovery pass both read the real clock.
func (p *preflightEnv) overdueMessage(t *testing.T) ids.UUID {
	t.Helper()
	now := time.Now()
	id := p.scheduleFor(t, now.Add(2*time.Hour))
	p.setDueAt(t, id, now.Add(-3*time.Hour))
	return id
}

// Nothing works the alarms here, so every alarm the first pass queues stays
// live. The second pass must reach the messages that did not fit in the first.
func TestARecoveryPassReachesMessagesBehindABatchWhoseAlarmsAreLive(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	source := p.overdueMessage(t)
	copies := p.cloneOverdue(t, source, compose.ScheduledSendRecoveryBatchForTest+4)

	for pass := 1; pass <= 2; pass++ {
		if err := p.runRecovery(t); err != nil {
			t.Fatalf("recovery pass %d: %v", pass, err)
		}
	}

	var unarmed, doubled int
	for _, id := range copies {
		switch n := p.alarmsFor(t, id); {
		case n == 0:
			unarmed++
		case n > 1:
			doubled++
		}
	}
	if unarmed != 0 || doubled != 0 {
		t.Fatalf("after two passes over %d overdue messages, %d have no alarm and %d have more than one; want exactly one each",
			len(copies)+1, unarmed, doubled)
	}
}

// The stamp is cleared between the passes, so only the alarm's uniqueness can
// stop the second pass from queueing a second alarm.
func TestARecoveryPassAddsNoAlarmBesideALiveOne(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	id := p.overdueMessage(t)
	scheduled := p.alarmsFor(t, id)

	if err := p.runRecovery(t); err != nil {
		t.Fatalf("first recovery pass: %v", err)
	}
	p.forgetRearm(t, id)
	if err := p.runRecovery(t); err != nil {
		t.Fatalf("second recovery pass: %v", err)
	}

	if got := p.alarmsFor(t, id); got != scheduled+1 {
		t.Fatalf("two passes left %d alarms for a message scheduled with %d, want %d: one re-arm, coalesced",
			got, scheduled, scheduled+1)
	}
}

// An alarm still waits for the later moment, so a key on the message alone
// would swallow the alarm for the earlier one.
func TestAMessageMovedEarlierGetsAnAlarmForItsNewMoment(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	now := time.Now()
	id := p.scheduleFor(t, now.Add(3*time.Hour))

	p.rescheduleTo(t, id, now.Add(time.Hour))

	if got := p.alarmsFor(t, id); got != 2 {
		t.Fatalf("a message moved earlier has %d alarms, want 2: the old moment's and the new one's", got)
	}
}
