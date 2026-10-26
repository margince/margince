// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func runGuestAnswersTask(t *testing.T, e *integration.Env) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{meetingGuestAnswersDataTask(e.Pool)}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// A calendar's rows written before answers were kept take each answer from the
// stored event, once. A row another door wrote is not touched.
func TestTheAnswerTaskFillsAnswersFromTheStoredEvent(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	grantContactScope(t, e, e.Rep1, "all")
	meeting := seedMatchedMeeting(t, e, conn, e.Rep1, rep1Address, matchedEventPayload(t, rep1Address, "client@acme.test", matchedGuests...))
	runMatchedMeetingTask(t, e)
	e.WsExec(t, `UPDATE activity_participant SET response = NULL WHERE activity_id = $1`, meeting)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'imported@acme.test', 'attendee')`, meeting)

	runGuestAnswersTask(t, e)

	for address, want := range map[string]string{"client@acme.test": "accepted", "buyer@acme.test": "needs_action"} {
		if got := e.WsScalar(t, `SELECT coalesce(string_agg(coalesce(response, '-'), ','), '') FROM activity_participant
			WHERE activity_id = $1 AND address = $2`, meeting, address); got != want {
			t.Errorf("%s answers %q, want %q", address, got, want)
		}
	}
	if got := e.WsScalar(t, `SELECT coalesce(response, '-') FROM activity_participant
		WHERE activity_id = $1 AND address = 'imported@acme.test'`, meeting); got != "-" {
		t.Errorf("a row the calendar did not write answers %q, want none", got)
	}
	// A second pass over the same originals finds nothing left to fill.
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	var again datatask.BatchOut
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		again, err = fillGuestAnswersBatch(ctx, tx, identity.NewService(e.Pool), datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if again.Scanned == 0 || again.Changed != 0 {
		t.Errorf("a second pass scanned %d originals and changed %d, want the original seen and nothing changed", again.Scanned, again.Changed)
	}
}
