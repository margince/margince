// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Two seats' first imports of one message, each recomputing the audience in
// its own open transaction. Each import row's foreign key holds a key-share
// lock on the activity until its transaction ends.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTwoSeatsFirstImportsRecomputeOneMessageAndTheStricterSeatWins(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	env.sync(t, customerMail("import-race@acme.example"))
	activityID := oneActivityID(t, e)
	// Both seats' rows are written inside the race, as two first syncs write them.
	if _, err := e.Owner.Exec(context.Background(),
		`DELETE FROM capture_import WHERE activity_id = $1`, activityID); err != nil {
		t.Fatalf("clearing the sync's import row: %v", err)
	}

	held := beginImport(t, e, activityID, e.Rep1, "held")
	shared := beginImport(t, e, activityID, e.Rep3, "shared")
	held.recompute()
	awaitRecomputeDoneOrQueued(t, e, held, shared.pid, "the held seat's recompute")
	shared.recompute()
	awaitRecomputeDoneOrQueued(t, e, shared, held.pid, "the shared seat's recompute")
	held.release()
	shared.release()

	for _, seat := range []*racingImport{held, shared} {
		if err := seat.wait(); err != nil {
			t.Fatalf("a seat's import failed beside the other's: %v", err)
		}
	}
	// The shared seat derived after the held seat committed, so it must see the hold.
	if got, why := audienceOf(t, e, activityID); got != "participants" || why != activities.ReasonPosture {
		t.Fatalf("audience = %q (%q), want participants (%q) from the held seat's import row",
			got, why, activities.ReasonPosture)
	}
}

// racingImport is one seat's transaction: its import row is written and the
// transaction stays open until the test lets it recompute and then commit.
type racingImport struct {
	pid        int32
	start      chan struct{}
	commit     chan struct{}
	recomputed chan struct{}
	finished   chan struct{}
	err        error
}

func beginImport(t *testing.T, e *integration.SearchEnv, activityID, seat ids.UUID, posture string) *racingImport {
	t.Helper()
	r := &racingImport{
		start: make(chan struct{}), commit: make(chan struct{}),
		recomputed: make(chan struct{}), finished: make(chan struct{}),
	}
	written := make(chan error, 1)
	ctx := principal.WithCorrelationID(e.Admin(), ids.NewV7())
	go func() {
		defer close(r.finished)
		r.err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&r.pid)
			if err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id, posture_at_import)
					VALUES ($1, $2, $3)`, activityID, seat, posture)
			}
			written <- err
			if err != nil {
				return err
			}
			<-r.start
			err = activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](activityID))
			close(r.recomputed)
			if err != nil {
				return err
			}
			<-r.commit
			return nil
		})
	}()
	select {
	case err := <-written:
		if err != nil {
			t.Fatalf("writing seat %s's import row: %v", seat, err)
		}
	case <-r.finished:
		t.Fatalf("seat %s's transaction ended before writing its import row: %v", seat, r.err)
	}
	t.Cleanup(func() {
		r.recompute()
		r.release()
		<-r.finished
	})
	return r
}

func (r *racingImport) recompute() { r.closeOnce(r.start) }

func (r *racingImport) release() { r.closeOnce(r.commit) }

func (r *racingImport) closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func (r *racingImport) wait() error {
	<-r.finished
	return r.err
}

// awaitRecomputeDoneOrQueued returns once seat's recompute has returned or
// some backend waits on holder, whichever the lock modes produce.
func awaitRecomputeDoneOrQueued(t *testing.T, e *integration.SearchEnv, seat *racingImport, holder int32, racer string) {
	t.Helper()
	testdb.WaitForContention(t, nil, "",
		racer+" neither returned nor waited on the other seat within the budget",
		func(ctx context.Context) (bool, error) {
			select {
			case <-seat.recomputed:
				return true, nil
			default:
			}
			queued, err := testdb.QueuedBehind(ctx, e.Owner, holder)
			return queued > 0, err
		})
}

// A colleague's import of a meeting holds a key share on its row, then
// recomputes the audience. A second seat's replay of the meeting takes the row
// lock before its own import row. So it waits for the colleague, and the
// recompute never waits on it.
func TestAMeetingReplayWaitsForAColleaguesImportBeforeItSharesTheRow(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	p.book(start)
	p.sync(t, "a")
	e := p.env.e
	colleague := beginImport(t, e, p.rowOf(t, "a"), e.Rep3, "shared")

	synced, returned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		synced <- p.env.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), e.WS), p.conns["b"])
	}()
	testdb.AwaitQueuedBehind(t, e.Owner, colleague.pid, 1, returned, "the guest's replay")
	colleague.recompute()
	testdb.AwaitReturnWithoutWaiting(t, e.Owner, colleague.pid, colleague.recomputed,
		"the colleague's recompute, beside the replay waiting on the colleague,")
	colleague.release()
	if err := colleague.wait(); err != nil {
		t.Fatalf("the colleague's import: %v", err)
	}
	if err := <-synced; err != nil {
		t.Fatalf("SyncOnce b: %v", err)
	}
	p.wantOneMeeting(t, start)
}
