// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"errors"

	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// A mail may itself be titled "Erased", so the subject cannot mark a reminder
// that erasure already reached.
func TestErasingAMessageErasesTheReminderThatQuotesIt(t *testing.T) {
	t.Parallel()
	for _, title := range []string{"Re: Angebot", erasedActivitySubject} {
		t.Run(title, func(t *testing.T) {
			env := setupSARIdentifiers(t)
			message := seedSubjectActivity(t, env)
			reminder := seedRequestReminder(t, env.owner, message, title)
			if err := erasePurgingContent(env, message); err != nil {
				t.Fatal(err)
			}
			assertReminderErased(t, env, reminder)
		})
	}
}

// Capture files a reminder while it holds the source row. An erasure that
// starts inside that window must still reach the reminder once it commits.
func TestErasingAMessageReachesTheReminderCaptureIsFiling(t *testing.T) {
	t.Parallel()
	env := setupSARIdentifiers(t)
	message := seedSubjectActivity(t, env)
	capture, err := env.owner.Begin(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := capture.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the capture transaction: %v", err)
		}
	}()
	mustExec(env.ctx, t, capture, `SELECT 1 FROM activity WHERE id = $1 FOR UPDATE`, message)
	reminder := seedRequestReminder(t, capture, message, "Re: Angebot")

	done := make(chan error, 1)
	go func() { done <- erasePurgingContent(env, message) }()
	waitForErasureToQueue(t, env, done)
	if err := capture.Commit(env.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	assertReminderErased(t, env, reminder)
}

func seedRequestReminder(t *testing.T, db execer, message ids.UUID, title string) ids.UUID {
	t.Helper()
	reminder := ids.NewV7()
	quoted := &systemtext.Message{
		Version: 1, Subject: systemtext.Sentence("task.reply_to_email"),
		Body: systemtext.Sentence("request.asked", systemtext.Text("Anna Weber"), systemtext.Text("Please send the report.")),
	}
	mustExec(context.Background(), t, db, `INSERT INTO activity
		(id, kind, subject, body, system_text, occurred_at, source, captured_by, source_system, source_id, source_activity_id)
		VALUES ($1, 'task', $5, 'Anna Weber asked: "Please send the report."', $6, now(), 'manual',
		        'system:test', $2, $3, $4)`, reminder, provenance.EmailRequestSource, message.String(), message, title, quoted)
	return reminder
}

func erasePurgingContent(env *sarIdentifierEnv, message ids.UUID) error {
	eraser := NewEraser(env.db).WithRawCapturePurger(&nilRawPurger{})
	return env.db.Tx(env.ctx, func(tx pgx.Tx) error {
		return eraser.purgeContentDerivedFrom(env.ctx, tx, message, theClockRanOut)
	})
}

// waitForErasureToQueue returns once the erasure waits on a lock or has
// finished; either way the window under test has been entered.
func waitForErasureToQueue(t *testing.T, env *sarIdentifierEnv, done chan error) {
	t.Helper()
	probe, err := pgx.Connect(env.ctx, testdb.For(t).OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := probe.Close(context.Background()); err != nil {
			t.Errorf("closing the probe connection: %v", err)
		}
	}()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	giveUp := time.NewTimer(10 * time.Second)
	defer giveUp.Stop()
	for {
		select {
		case <-giveUp.C:
			t.Fatal("the erasure neither queued behind capture nor finished within ten seconds")
		case <-tick.C:
		}
		if len(done) > 0 {
			return
		}
		var queued int
		if err := probe.QueryRow(env.ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&queued); err != nil {
			t.Fatal(err)
		}
		if queued > 0 {
			return
		}
	}
}

func assertReminderErased(t *testing.T, env *sarIdentifierEnv, reminder ids.UUID) {
	t.Helper()
	var subject string
	var bodyLeft, marked bool
	if err := env.owner.QueryRow(context.Background(),
		`SELECT subject, body IS NOT NULL, system_text = $2 FROM activity WHERE id = $1`, reminder, erasedActivityText()).
		Scan(&subject, &bodyLeft, &marked); err != nil {
		t.Fatal(err)
	}
	if subject != erasedActivitySubject || bodyLeft || !marked {
		t.Errorf("the reminder kept subject %q (body left: %v, system text erased: %v) after its message was "+
			"erased; it quotes the sender's words and must go with them", subject, bodyLeft, marked)
	}
}
