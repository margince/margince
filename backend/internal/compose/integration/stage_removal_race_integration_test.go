// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The occupancy guard against genuinely concurrent writers. The refusal
// is only a guard if the stage a deal is being bound to is LOCKED by the
// binding write: as a plain read, a deal can resolve a live stage, the
// removal can count zero and archive it, and the deal's own write still
// lands — the FK on deal.stage_id asks whether the row exists, and
// archiving is exactly the operation that leaves it existing.
//
// The invariant asserted is the one that must hold whichever side wins:
// no LIVE deal ever sits on an archived stage. A deal that reached one
// is invisible on every board read, all of which filter to live stages.
//
// Both orderings are PINNED rather than raced for. Left to a bare
// WaitGroup the interleaving is the scheduler's to choose, so whether the
// race ran at all was the machine's decision and not the test's — on a
// host where one side consistently won, this suite failed at its own
// anti-vacuity guard, correctly refusing to report a pass it had not
// earned. Each ordering here is instead held open by a row this file
// locks from its own connection, and the lock the winner takes is
// asserted DIRECTLY with a NOWAIT probe. That turns "the race never ran"
// from a flake into an impossible state: a lookup that failed to lock is
// a failure with its own sentence, not a round that silently did nothing.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNoDealLandsOnAStageBeingRemoved(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)

	t.Run("the advance holds the stage the removal wants", func(t *testing.T) {
		theAdvanceReachesTheStageFirst(t, e, pipeline, open)
	})
	t.Run("the removal holds the stage the advance wants", func(t *testing.T) {
		theRemovalReachesTheStageFirst(t, e, pipeline, open)
	})

	// The invariant, over everything both orderings produced.
	if n := e.WsCount(t, `SELECT count(*) FROM deal d JOIN stage s ON s.id = d.stage_id
	                      WHERE d.archived_at IS NULL AND s.archived_at IS NOT NULL`); n != 0 {
		t.Fatalf("%d live deal(s) sit on a removed stage — the occupancy count was read, not held", n)
	}
}

// theAdvanceReachesTheStageFirst pins the ordering in which the deal's move
// gets to the stage first: the removal must then find the deal it had just
// gained and refuse on occupancy.
//
// The advance is held open by the DEAL row, which it patches only after its
// target lookup has taken the stage — so while it waits there, the lock the
// removal has to queue behind is provably held, and the probe below says so
// rather than hoping for it.
func theAdvanceReachesTheStageFirst(t *testing.T, e *Env, pipeline ids.PipelineID, open ids.StageID) {
	admin := e.Admin()
	target := raceStage(t, e, pipeline, "Racy: advance first", 90)
	deal := e.SeedDeal(t, "Racer: advance first", pipeline, open, &e.Rep1)

	release := holdRow(t, holdDealRow, deal)

	advance := make(chan error, 1)
	advanceReturned := make(chan struct{})
	go func() {
		defer close(advanceReturned)
		_, err := e.Deals.AdvanceDeal(admin, ids.From[ids.DealKind](deal),
			deals.AdvanceDealInput{ToStageID: target})
		advance <- err
	}()

	waitUntilRowLockIsRefused(t, probeStageForUpdate, target.UUID, advanceReturned,
		"the advance returned before it ever held the target stage — nothing in this round could have blocked the removal",
		"the advance never took a lock on the target stage: its lookup reads the stage without holding it, "+
			"which is exactly the window that lets a removal archive a stage a deal is landing on")

	// The removal now queues behind that lock. Whether it reaches it before
	// or after the release below does not change the outcome — once the deal
	// is committed onto the stage, occupancy refuses the removal either way —
	// and the lock itself is already proved.
	remove := make(chan error, 1)
	go func() { remove <- e.Deals.ArchiveStage(admin, target, nil) }()
	release()

	if err := <-advance; err != nil {
		t.Fatalf("the advance held the stage and still did not land the deal: %v", err)
	}
	var occupied *deals.StageOccupiedError
	if err := <-remove; !errors.As(err, &occupied) {
		t.Fatalf("the removal ran against a stage the advance had just occupied and answered %v, "+
			"not the occupancy refusal", err)
	}
}

// theRemovalReachesTheStageFirst pins the mirror ordering: the removal takes
// the stage first, and the advance — which waits on that same lock — must be
// re-evaluated against the committed archival and find no live stage, rather
// than binding the deal to the snapshot it read before waiting.
//
// The removal is held open by a BYSTANDER stage, which it renumbers only
// after it has locked and archived the target.
func theRemovalReachesTheStageFirst(t *testing.T, e *Env, pipeline ids.PipelineID, open ids.StageID) {
	admin := e.Admin()
	target := raceStage(t, e, pipeline, "Racy: removal first", 91)
	bystander := raceStage(t, e, pipeline, "Bystander", 92)
	deal := e.SeedDeal(t, "Racer: removal first", pipeline, open, &e.Rep1)

	release := holdRow(t, holdStageRow, bystander.UUID)

	remove := make(chan error, 1)
	removeReturned := make(chan struct{})
	go func() {
		defer close(removeReturned)
		remove <- e.Deals.ArchiveStage(admin, target, nil)
	}()

	waitUntilRowLockIsRefused(t, probeStageForKeyShare, target.UUID, removeReturned,
		"the removal returned before it ever held the target stage — nothing in this round could have blocked the advance",
		"the removal never took an exclusive lock on the target stage, so a deal's target lookup could pass "+
			"straight through the archival it is meant to wait for")

	advance := make(chan error, 1)
	go func() {
		_, err := e.Deals.AdvanceDeal(admin, ids.From[ids.DealKind](deal),
			deals.AdvanceDealInput{ToStageID: target})
		advance <- err
	}()
	release()

	if err := <-remove; err != nil {
		t.Fatalf("the removal held an empty stage and still did not archive it: %v", err)
	}
	if err := <-advance; !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the advance waited on a stage that was archived underneath it and answered %v, "+
			"not the missing-stage refusal", err)
	}
}

// raceStage adds one live stage to the pipeline for a round to contend over.
func raceStage(t *testing.T, e *Env, pipeline ids.PipelineID, name string, position int) ids.StageID {
	t.Helper()
	stage, err := e.Deals.CreateStage(e.Admin(), deals.CreateStageInput{
		PipelineID: pipeline, Name: name, Position: position, Semantic: "open",
	})
	if err != nil {
		t.Fatalf("seeding the contested stage %q: %v", name, err)
	}
	return ids.From[ids.StageKind](ids.UUID(stage.Id))
}

// The rows a round holds to pin its ordering, and the probes that ask whether
// the racer has taken the lock its counterpart must wait on. Constants because
// the table is this file's own choice and never travels through a parameter.
const (
	holdDealRow  = `SELECT 1 FROM deal WHERE id = $1 FOR UPDATE`
	holdStageRow = `SELECT 1 FROM stage WHERE id = $1 FOR UPDATE`
	// FOR UPDATE conflicts with the FOR KEY SHARE an advance's target lookup
	// takes, and FOR KEY SHARE conflicts with the FOR UPDATE a removal takes —
	// so each probe is refused by exactly the racer it is asking about, and by
	// no weaker lock a concurrent rename or probability edit might hold.
	probeStageForUpdate   = `SELECT 1 FROM stage WHERE id = $1 FOR UPDATE NOWAIT`
	probeStageForKeyShare = `SELECT 1 FROM stage WHERE id = $1 FOR KEY SHARE NOWAIT`
)

// lockNotAvailable is what PostgreSQL answers a NOWAIT request that would have
// waited — the probe's whole signal, so it is matched on the code rather than
// on a message that carries the row's identity and changes with it.
const lockNotAvailable = "55P03"

// waitUntilRowLockIsRefused waits until the contested row is held hard enough
// to refuse this probe, which is what proves the racer reached it.
//
// The look is the whole of this suite's contribution; the loop around it —
// budget, pacing, and the rule that a probe which gave up says what the run
// failed to prove — is testdb.WaitForContention, shared with the
// pg_stat_activity probe that asks a different question in the same shape.
func waitUntilRowLockIsRefused(
	t *testing.T, query string, id ids.UUID, racerReturned <-chan struct{}, finishedEarly, missed string,
) {
	t.Helper()
	conn := freshConnection(t)
	testdb.WaitForContention(t, racerReturned, finishedEarly, missed, func(ctx context.Context) (bool, error) {
		// The probe's own lock is held for the statement and no longer, so a
		// look taken before the racer arrives delays it by a round trip at
		// most and never changes which side wins.
		_, err := conn.Exec(ctx, query, id)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == lockNotAvailable {
			return true, nil
		}
		return false, err
	})
}

// holdRow pins one row until the returned release runs: a connection of this
// test's own holds it FOR UPDATE, so a racer that must write it waits there
// instead of finishing, and the ordering under test stays open long enough to
// be observed.
//
// The release is idempotent and also registered as cleanup, because a round
// that fails an assertion returns without reaching its own release — and a
// transaction left holding a row outlives the test that opened it and blocks
// whatever runs next in the package.
func holdRow(t *testing.T, query string, id ids.UUID) func() {
	t.Helper()
	ctx := context.Background()
	conn := freshConnection(t)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the transaction that holds the contested row: %v", err)
	}
	if _, err := tx.Exec(ctx, query, id); err != nil {
		t.Fatalf("holding the contested row: %v", err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the contested row: %v", err)
		}
	}
	t.Cleanup(release)
	return release
}

// freshConnection dials a connection outside the stores' pool. The pool is
// what the racing goroutines draw on, so borrowing from it to hold a row or to
// probe one would contend for the very connections the race needs.
func freshConnection(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("dialling this round's own connection: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing this round's own connection: %v", err)
		}
	})
	return conn
}
