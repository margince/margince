// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordsources

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// olderActivity is one activity as written before the source check, with the
// channel and the system the correction should leave it on.
type olderActivity struct {
	kind, source, capturedBy string
	system, sourceID         *string
	origin                   [2]string
}

func (a olderActivity) plant(t *testing.T, owner *pgx.Conn) ids.UUID {
	t.Helper()
	return integration.SeedBeforeSourceCheck(t, owner, "activity",
		`INSERT INTO activity (id, kind, subject, source, source_system, source_id, captured_by)
		 VALUES ($1, $2, 'Older entry', $3, $4, $5, $6)`, a.kind, a.source, a.system, a.sourceID, a.capturedBy)
}

// Each older word lands on its channel. The product's own writer moves to
// source_system where that column is free: not on a task, and not beside a
// source_id. The system's own word names no writer.
func TestTheTaskMovesOlderActivitiesOntoAChannel(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	sla, mail, transcript, dispact, hubspot, csv := "lead_sla", "email", "transcript", "dispact", "hubspot", "mirror:csv:bundle"
	msg, doc, talk, engagement, planted := "msg-1", "https://docs.test/notes", "talk:1", "e-1", "planted-1"
	rows := []olderActivity{
		{"task", "system", "system", nil, nil, [2]string{"automation", ""}},
		{"task", "commitment", "human:x", nil, nil, [2]string{"automation", ""}},
		{"email", "system", "system:comms-send", nil, nil, [2]string{"automation", ""}},
		{"note", "attachment_access_request", "human:x", nil, nil, [2]string{"automation", "attachment_access_request"}},
		{"task", "system", "system:lead-sla", &sla, nil, [2]string{"automation", sla}},
		{"note", "", "system", nil, nil, [2]string{"automation", ""}},
		{"note", "deal_room", "system", nil, nil, [2]string{"automation", "deal_room"}},
		{"meeting", "meeting_notes", "connector:gdocs", &transcript, &doc, [2]string{"capture", transcript}},
		{"email", "gmail:" + msg, "connector:gmail", &mail, &msg, [2]string{"capture", mail}},
		{"note", "public_booking", "system:public_booking", nil, nil, [2]string{"webform", "public_booking"}},
		{"note", "chat_links", "system:chat_links", &dispact, &talk, [2]string{"automation", dispact}},
		{"note", "ui", "human:x", nil, nil, [2]string{"manual", ""}},
		{"note", "hubspot_import:1", "human:x", &hubspot, &engagement, [2]string{"api", hubspot}},
		{"note", "mirror:csv:bundle:activity:7", "human:x", &csv, &engagement, [2]string{"import", csv}},
		{"note", "system", "human:x", nil, &planted, [2]string{"automation", ""}},
	}
	rowIDs := make([]ids.UUID, len(rows))
	for i, row := range rows {
		rowIDs[i] = row.plant(t, owner)
	}

	ctx := systemCtx(e)
	task := ActivityDataTask()
	if n, capped, err := task.Steps[0].Estimate(ctx, e.Pool, 3); err != nil || n != 3 || !capped {
		t.Fatalf("the activity estimate under a cap of 3 = %d, capped %v (%v), want 3 and capped", n, capped, err)
	}
	e.RunDataTask(ctx, t, task)

	for i, row := range rows {
		if got := partyOrigin(t, e, "activity", rowIDs[i]); got != row.origin {
			t.Errorf("a %s with source %q reads %v, want %v", row.kind, row.source, got, row.origin)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND action = 'update'
		AND before ? 'source' AND evidence ? 'data_task_run'`); n != len(rows) {
		t.Errorf("%d activity audit row(s) keep an older source, want %d", n, len(rows))
	}
	if n, _, err := task.Steps[0].Estimate(ctx, e.Pool, 10); err != nil || n != 0 {
		t.Errorf("the activity step still estimates %d row(s) (%v), want none", n, err)
	}
}

// A restricted activity refuses every update, so the task leaves it and does
// not count it. It is moved when the restriction is lifted.
func TestTheTaskLeavesARestrictedActivity(t *testing.T) {
	e := integration.Setup(t)
	restricted := integration.SeedBeforeSourceCheck(t, integration.OwnerConn(t), "activity",
		`INSERT INTO activity (id, kind, subject, source, captured_by, archived_at, retention_class, retention_class_at,
		                       restricted_at, restricted_reason, restricted_until)
		 VALUES ($1, 'email', 'Held mail', 'gmail:msg-9', 'connector:gmail', now(), 'commercial_correspondence', now(),
		         now(), 'statutory retention', now() + interval '6 years')`)
	ctx := systemCtx(e)
	task := ActivityDataTask()
	if n, _, err := task.Steps[0].Estimate(ctx, e.Pool, 10); err != nil || n != 0 {
		t.Fatalf("the activity step estimates %d row(s) (%v), want the restricted one left out", n, err)
	}
	e.RunDataTask(ctx, t, task)
	if got := e.WsScalar(t, `SELECT source FROM activity WHERE id = $1`, restricted); got != "gmail:msg-9" {
		t.Errorf("the restricted activity reads source %q, want it untouched", got)
	}
}

// An older activity is judged by the unvalidated check on its next write. An
// edit before the task goes through and moves the row onto its channel.
func TestAnEditBeforeTheTaskMovesAnOlderActivityOntoItsChannel(t *testing.T) {
	e := integration.Setup(t)
	older := olderActivity{"note", "deal_room", "human:x", nil, nil, [2]string{}}.plant(t, integration.OwnerConn(t))
	subject := "Renamed entry"
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](older), activities.UpdateActivityInput{Subject: &subject}); err != nil {
		t.Fatalf("editing an older activity: %v", err)
	}
	if got, want := partyOrigin(t, e, "activity", older), [2]string{"automation", "deal_room"}; got != want {
		t.Errorf("the edited activity reads %v, want %v", got, want)
	}
}
