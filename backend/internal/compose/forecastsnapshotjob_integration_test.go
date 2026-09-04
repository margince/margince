// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The daily forecast snapshot against a real database.
//
// It has to be an integration test rather than a unit one, because what was
// missing was not a function — TakeSnapshot has been there and tested all along
// — but a PRODUCER wired to it. What is asserted here is the whole path the
// pass takes: resolve the installation's period, read the deals inside it under
// the fleet's own principal, compute the readings and freeze them, with the
// contribution rows a movement diff later reads.
//
// The second case is the one the design turns on. The dispatcher ticks more
// than once a day so a worker that was down still backfills, and River retries
// a failed attempt — so a second run in one local day is ordinary rather than
// exceptional, and the daily arbiter is a UNIQUE INDEX. A pass that let that
// surface as an error would fail loudly every day after the first success.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// snapshotJobEnv is one workspace holding a deal that falls inside the period
// the pass will freeze.
type snapshotJobEnv struct {
	*integration.Env
	worker *forecastSnapshotWorkspaceWorker
	at     time.Time
}

func setupSnapshotJob(t *testing.T) *snapshotJobEnv {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	at := time.Now().UTC()

	// A deal with money and a close date inside the current quarter, so the
	// readings the pass freezes are not all zero — a snapshot of an empty
	// pipeline would be written by a pass that read nothing at all, and this
	// suite could not tell the two apart.
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by,
		                  amount_minor, currency, expected_close_date)
		VALUES ($1, $2, 'Snapshot Fixture', $3, 'open', 'manual', 'test', 4200000, 'EUR', $4)`,
		pipeline, open, e.Rep1, at.AddDate(0, 0, 3)); err != nil {
		t.Fatalf("seeding the deal the snapshot sums: %v", err)
	}
	return &snapshotJobEnv{
		Env: e,
		worker: &forecastSnapshotWorkspaceWorker{
			pool: e.Pool,
			now:  func() time.Time { return at },
			log:  slog.New(slog.DiscardHandler),
		},
		at: at,
	}
}

// run drives the worker exactly as River would.
func (e *snapshotJobEnv) run(t *testing.T) error {
	t.Helper()
	return e.worker.Work(context.Background(), &river.Job[ForecastSnapshotWorkspaceArgs]{
		JobRow: &rivertype.JobRow{},
		Args:   ForecastSnapshotWorkspaceArgs{Workspace: e.WS},
	})
}

// dailySnapshots counts what the pass has frozen, and answers the one id there
// is when there is one.
func (e *snapshotJobEnv) dailySnapshots(t *testing.T) (int, ids.UUID) {
	t.Helper()
	owner := integration.OwnerConn(t)
	rows, err := owner.Query(context.Background(), `
		SELECT id FROM forecast_snapshot
		 WHERE trigger = $1 AND scope_kind = $2 AND scope_id IS NULL
		 ORDER BY taken_at`, forecasting.TriggerDaily, forecasting.ScopeWorkspace)
	if err != nil {
		t.Fatalf("reading the frozen snapshots: %v", err)
	}
	defer rows.Close()
	var frozen []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		frozen = append(frozen, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(frozen) == 0 {
		return 0, ids.Nil
	}
	return len(frozen), frozen[0]
}

// The producer three shipped surfaces were waiting for: snapshot shares point
// at a frozen state, the movement waterfall differences two of them, and the
// shared CSV export serves one. Before this pass, nothing outside a test wrote
// the table they all read.
func TestTheDailyPassFreezesTheWorkspacesForecast(t *testing.T) {
	e := setupSnapshotJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the daily pass: %v", err)
	}

	count, id := e.dailySnapshots(t)
	if count != 1 {
		t.Fatalf("the pass froze %d workspace snapshots, want exactly one", count)
	}
	// Read back through the reader a movement diff uses, not by re-querying the
	// columns: what matters is that the frozen state is one this product can
	// actually difference, and the contributions are what make a headline
	// answerable at all.
	store := forecasting.NewStore(InstallationDB(e.Pool))
	var side struct {
		contributions int
	}
	if err := store.InTx(e.As(e.AdminUser, nil, integration.AdminPerms),
		func(ctx context.Context, tx pgx.Tx) error {
			read, err := store.SnapshotSide(ctx, tx, id)
			if err != nil {
				return err
			}
			side.contributions = len(read.Contributions)
			return nil
		}); err != nil {
		t.Fatalf("reading the snapshot back: %v", err)
	}
	if side.contributions != 1 {
		t.Errorf("the snapshot carries %d contributions over a one-deal period — a frozen total "+
			"with no rows behind it can only say that a number changed", side.contributions)
	}
}

// The dispatcher ticks more than once a day and River retries, so a second run
// inside one local day is ordinary. The arbiter is a unique index, and a pass
// that let it surface as an error would fail every day after the first success
// — loudly, and about the outcome it wanted.
func TestASecondPassInOneDayFreezesNothingAndSucceeds(t *testing.T) {
	e := setupSnapshotJob(t)
	if err := e.run(t); err != nil {
		t.Fatalf("the first pass: %v", err)
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the second pass in one local day reported %v — the arbiter refusing a duplicate "+
			"is this pass succeeding, not failing", err)
	}

	if count, _ := e.dailySnapshots(t); count != 1 {
		t.Errorf("two passes in one local day left %d snapshots, want the one the arbiter admits", count)
	}
}
