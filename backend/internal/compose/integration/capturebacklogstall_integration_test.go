// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// When a seat is told their capture backlog stopped moving, and when they are
// not.
//
// This notice is the only thing that says so. A row that SPENDS its attempts
// retires to `unsure` and reaches a human through the review queue — but an
// outage REFUNDS the attempt rather than spending it, deliberately, so a
// provider being down does not retire rows for reasons that had nothing to do
// with the question. The consequence is that during a real stall nothing
// exhausts, nothing retires, and the seat's mail sits withheld with no sign of
// it anywhere.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASeatWhoseBacklogStoppedMovingIsFound(t *testing.T) {
	e := Setup(t)
	seedPendingSender(t, e, e.Rep1, "wartet@example.test", 1, 8*time.Hour)

	stalled := stalledSeats(t, e)
	if stalled[e.Rep1] != 1 {
		t.Fatalf("stalled[%v] = %d, want 1 — this row arrived eight hours ago and is still "+
			"waiting for an answer", e.Rep1, stalled[e.Rep1])
	}
}

func TestAFreshlyArrivedRowIsNotStalled(t *testing.T) {
	// New, not stuck. Mail that just landed is pending because nothing has
	// reached it yet, and reporting that would tell every seat their backlog is
	// broken every time a message arrives.
	e := Setup(t)
	seedPendingSender(t, e, e.Rep1, "gerade@example.test", 0, time.Minute)

	if stalled := stalledSeats(t, e); len(stalled) != 0 {
		t.Fatalf("stalled = %v, want empty — this row arrived a minute ago", stalled)
	}
}

func TestARowTheRetryLoopKeepsTouchingIsStillStalled(t *testing.T) {
	// The case the first version of this predicate could not see, and the reason
	// it reads created_at. During an outage the pass re-claims and re-defers each
	// row every cycle, so updated_at and next_attempt_at are always fresh — a
	// predicate on either reports a healthy lane while the seat's mail sits.
	e := Setup(t)
	seedPendingSender(t, e, e.Rep1, "haengt@example.test", 1, 8*time.Hour)
	// What Defer does on every outage cycle.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET updated_at = now(), next_attempt_at = now() + interval '30 minutes'
			 WHERE email = $1`, "haengt@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if stalled := stalledSeats(t, e); stalled[e.Rep1] != 1 {
		t.Fatalf("stalled[%v] = %d, want 1 — the retry loop touching a row is not the lane "+
			"answering it", e.Rep1, stalled[e.Rep1])
	}
}

func TestAResolvedRowIsNotStalled(t *testing.T) {
	// The question was answered. Only a row still waiting is waiting.
	e := Setup(t)
	seedPendingSender(t, e, e.Rep1, "beantwortet@example.test", 2, 8*time.Hour)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_pending_counterparty SET status = 'real' WHERE email = $1`,
			"beantwortet@example.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if stalled := stalledSeats(t, e); len(stalled) != 0 {
		t.Fatalf("stalled = %v, want empty — this sender was judged", stalled)
	}
}

func TestTheStallIsCountedPerSeat(t *testing.T) {
	// Whose backlog stopped is the question the notice answers, so two seats
	// waiting are two notices rather than one number nobody can act on.
	e := Setup(t)
	seedPendingSender(t, e, e.Rep1, "eine@example.test", 1, 8*time.Hour)
	seedPendingSender(t, e, e.Rep1, "zwei@example.test", 1, 8*time.Hour)
	seedPendingSender(t, e, e.Rep2, "drei@example.test", 1, 8*time.Hour)

	stalled := stalledSeats(t, e)
	if stalled[e.Rep1] != 2 || stalled[e.Rep2] != 1 {
		t.Fatalf("stalled = %v, want two for the first seat and one for the colleague", stalled)
	}
}

func stalledSeats(t *testing.T, e *Env) map[ids.UUID]int {
	t.Helper()
	// The window every test here asks about: long enough that a row retried a
	// minute ago is still moving, short enough that one untouched for eight
	// hours is not.
	const quiet = 6 * time.Hour
	out, err := capture.NewPendingStore(e.DB()).StalledBacklogSeats(e.Admin(), quiet)
	if err != nil {
		t.Fatalf("finding stalled seats: %v", err)
	}
	return out
}

// seedPendingSender lands one unanswered disposition, with the two facts the
// stall predicate reads: how many times it has been asked about, and how long
// ago it arrived.
func seedPendingSender(t *testing.T, e *Env, owner ids.UUID, email string, attempts int, quietFor time.Duration) {
	t.Helper()
	activity := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source, captured_by,
			                      counterparty_email, audience)
			VALUES ($1, 'email', 'wartet', 'der Text', 'inbound', 'gmail', 'connector:gmail',
			        $2, 'participants')`, activity, email); err != nil {
			return err
		}
		// updated_at last, and as its own statement: the row's trigger stamps it
		// on insert, so a single INSERT cannot land a row that was touched hours
		// ago and the backdating has to follow.
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO capture_pending_counterparty
			       (email, domain, activity_id, owner_id, status, attempts, next_attempt_at)
			VALUES ($1, split_part($1, '@', 2), $2, $3, 'pending', $4, now())`,
			email, activity, owner, attempts); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_pending_counterparty SET created_at = now() - $2::interval WHERE email = $1`,
			email, quietFor.String())
		return err
	}); err != nil {
		t.Fatalf("seeding a pending sender: %v", err)
	}
}
