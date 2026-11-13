// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	overdueQueueSize = 5000
	overdueGrace     = 10 * time.Minute
)

// scheduleBerlinSend writes one pending message through the real writer, at a
// Berlin wall clock a day after draftClock.
func scheduleBerlinSend(t *testing.T, e *sendEnv) ids.UUID {
	t.Helper()
	zone, err := values.ParseTimezone("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	intent, err := values.ParseIntentTime("2026-09-02T11:00", zone)
	if err != nil {
		t.Fatal(err)
	}
	out, err := draftStore(e).SendOrSchedule(e.as(principal.RowScopeAll), FromActivity(e.seedAnchor(t, "", "")), sendInput("transactional"),
		&SendSchedule{Intent: &intent}, stubConsentGate{}, &recordingStager{}, &recordingTimer{})
	if err != nil || out.Scheduled == nil {
		t.Fatalf("SendOrSchedule = %+v, %v", out, err)
	}
	return out.Scheduled.ID
}

// cloneScheduledSend copies the written row once per offset from draftClock,
// keeping its Berlin wall clock in step with the copy's instant.
func cloneScheduledSend(t *testing.T, e *sendEnv, source ids.UUID, offsets string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO scheduled_send
		SELECT (jsonb_populate_record(s, jsonb_build_object(
		         'id', uuidv7(),
		         'scheduled_at', $2::timestamptz + offset_minutes * interval '1 minute',
		         'scheduled_wall', ($2::timestamptz + offset_minutes * interval '1 minute') AT TIME ZONE 'Europe/Berlin'))).*
		  FROM scheduled_send s, unnest($3::int[]) AS offset_minutes
		 WHERE s.id = $1`, source, draftClock, offsets); err != nil {
		t.Fatalf("cloning the scheduled message: %v", err)
	}
}

// staleCachedInstant moves a row's cached instant away from what its wall clock
// resolves to, the state a zone rule change leaves behind.
func staleCachedInstant(t *testing.T, e *sendEnv, id ids.UUID, wall, cached time.Time) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE scheduled_send SET scheduled_wall = $2, scheduled_at = $3 WHERE id = $1`,
		id, wall.In(berlin(t)).Format("2006-01-02T15:04:05"), cached); err != nil {
		t.Fatal(err)
	}
}

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// minuteOffsets is a Postgres int array literal of from, from+step, … below to.
func minuteOffsets(from, to, step int) string {
	var offsets []string
	for m := from; m < to; m += step {
		offsets = append(offsets, strconv.Itoa(m))
	}
	return "{" + strings.Join(offsets, ",") + "}"
}

func TestOverdueScheduledSendsReadsOnlyTheDueIndexRange(t *testing.T) {
	e := setupSend(t)
	ctx := context.Background()
	source := scheduleBerlinSend(t, e)
	cloneScheduledSend(t, e, source, minuteOffsets(60, 60+overdueQueueSize*60, 60))
	cloneScheduledSend(t, e, source, minuteOffsets(-600, -60, 60))

	ruleMovedEarlier, ruleMovedLater := scheduleBerlinSend(t, e), scheduleBerlinSend(t, e)
	staleCachedInstant(t, e, ruleMovedEarlier, draftClock.Add(-time.Hour), draftClock.Add(20*time.Hour))
	staleCachedInstant(t, e, ruleMovedLater, draftClock.Add(time.Hour), draftClock.Add(-time.Hour))
	if _, err := e.owner.Exec(ctx, `ANALYZE scheduled_send`); err != nil {
		t.Fatal(err)
	}

	overdue, err := draftStore(e).OverdueScheduledSends(e.as(principal.RowScopeAll), overdueGrace, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(overdue) != 10 || !slices.Contains(overdue, ruleMovedEarlier) || slices.Contains(overdue, ruleMovedLater) {
		t.Fatalf("overdue = %d rows (earlier=%v later=%v), want the nine past copies and the row its zone rules moved earlier",
			len(overdue), slices.Contains(overdue, ruleMovedEarlier), slices.Contains(overdue, ruleMovedLater))
	}

	var plan string
	if err := e.owner.QueryRow(ctx, `EXPLAIN (ANALYZE, FORMAT JSON) `+overdueScheduledSendsSQL,
		overdueScheduledSendsArgs(draftClock.Add(-overdueGrace), 100)).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	t.Logf("plan over %d queued messages: %s", overdueQueueSize, plan)
	var scan planNode
	decodePlan(t, plan).walk(func(n planNode) {
		if n.RelationName == "scheduled_send" && scan.RelationName == "" {
			scan = n
		}
	})
	read := scan.ActualRows + scan.RemovedByFilter
	if scan.IndexName != "idx_scheduled_send_due" || read > 60 {
		t.Fatalf("the overdue read scanned scheduled_send by %q via %q and read %.0f rows, want idx_scheduled_send_due over the next %s only",
			scan.NodeType, scan.IndexName, read, zoneRuleShift)
	}
}
