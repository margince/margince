// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pastTheHorizon is well beyond the compiled ninety days a workspace with too
// few answers to measure its own horizon falls back to.
var pastTheHorizon = requestInstant.AddDate(0, 0, 200)

// asSeat binds one seat at all row scope with every activity grant, the shape a
// rep taking or editing a request needs.
func (e *loadEnv) asSeat(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Create: true, Update: true, Delete: true},
				"contact":  {Read: true}, "deal": {Read: true}, "company": {Read: true}, "lead": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func (e *loadEnv) waitingAt(ctx context.Context, t *testing.T, asOf time.Time) map[ids.UUID]bool {
	t.Helper()
	rows, err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).WaitingReplies(ctx, asOf)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	out := map[ids.UUID]bool{}
	for _, row := range rows {
		out[row.ActivityID] = true
	}
	return out
}

// A classified request ages out like any other wait, and so does one only the
// system filed a reminder for; one a human took stays.
func TestOnlyAHumanKeepsARequestPastTheHorizon(t *testing.T) {
	e := setupLoad(t)
	colleague := e.asSeat(e.other)
	reminded := seedEmailRequest(t, e, "Please confirm the date", "meeting", OwedVerdictAsksUs)
	if err := storeKnowing(e).CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if e.waitingAt(colleague, t, pastTheHorizon)[reminded] {
		t.Fatal("a reminder only the system wrote kept its request past the horizon")
	}
	id := seedEmailRequest(t, e, "Please send the contract", "commitment", OwedVerdictAsksUs)

	if e.waitingAt(colleague, t, pastTheHorizon)[id] {
		t.Fatal("a request nobody holds is still daily work 200 days later")
	}
	request := LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &id}
	if _, _, err := storeKnowing(e).LogActivity(e.asSeat(e.rep), request); err != nil {
		t.Fatalf("taking the request: %v", err)
	}
	if !e.waitingAt(colleague, t, pastTheHorizon)[id] {
		t.Fatal("a request a human took aged out of the queue")
	}
}

// Mail on an open deal ages out too: the deal's own risk lane speaks for a
// silent deal, so the reply queue need not hold its history forever.
func TestMailOnAnOpenDealAgesOut(t *testing.T) {
	e := setupLoad(t)
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Aged renewal', 'open', $2, $3, $4, 'seed', 'system')`, deal, e.rep, pipeline, stage)
	activity := e.seedWait(t, "Old question on the deal", "deal_id", deal)
	e.exec(t, `UPDATE activity SET occurred_at = now() - interval '200 days' WHERE id = $1`, activity)

	if e.waitingAt(e.as(), t, time.Now())[activity] {
		t.Fatal("200-day-old mail on an open deal is still daily work")
	}
}

// The reminder the pass filed retires with its request; one a human worked stays.
func TestAnAgedSystemReminderRetiresAndAWorkedOneStays(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	untouched := seedEmailRequest(t, e, "Please send slots", "meeting", OwedVerdictAsksUs)
	worked := seedEmailRequest(t, e, "Please send the offer", "commitment", OwedVerdictAsksUs)
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	reminderOf := func(source ids.UUID) (ids.UUID, bool) {
		var id ids.UUID
		var archived bool
		if err := e.owner.QueryRow(context.Background(),
			`SELECT id, archived_at IS NOT NULL FROM activity WHERE source_activity_id = $1`, source).Scan(&id, &archived); err != nil {
			t.Fatalf("reading the reminder of %s: %v", source, err)
		}
		return id, archived
	}
	workedReminder, _ := reminderOf(worked)
	renamed := "Send the offer by Friday"
	if _, err := store.UpdateActivity(e.asSeat(e.rep), ids.From[ids.ActivityKind](workedReminder),
		UpdateActivityInput{Subject: &renamed}); err != nil {
		t.Fatalf("a human renaming the reminder: %v", err)
	}

	if err := store.CaptureEmailRequests(asClassifier(e), pastTheHorizon); err != nil {
		t.Fatal(err)
	}
	if _, archived := reminderOf(untouched); !archived {
		t.Fatal("the system's reminder outlived its request's horizon")
	}
	if _, archived := reminderOf(worked); archived {
		t.Fatal("a reminder a human worked was retired by age")
	}
	// The read side of the same rule: the request behind a worked reminder is
	// still work for a colleague, and the untouched one is not.
	colleague := e.asSeat(e.other)
	still := e.waitingAt(colleague, t, pastTheHorizon)
	if !still[worked] {
		t.Error("the request whose reminder a human worked left the queue")
	}
	if still[untouched] {
		t.Error("the request whose reminder nobody touched is still in the queue")
	}
	var done bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT is_done FROM activity WHERE source_activity_id = $1`, untouched).Scan(&done); err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("an aged reminder was completed, which would settle a request nobody answered")
	}
}
