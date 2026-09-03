// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package forecasting

// The snapshot's constraints live in SQL, so a unit test over this package's Go
// cannot see them at all.
//
// Three of them are the kind that only fail on a row a real population
// produces: an amount and its currency travel together, a converted amount
// names the rate it was converted at, and a weighted amount exists exactly when
// there is a base amount to weight. The writer builds every one of those pairs
// itself, so a change to it can start producing rows the table refuses — and
// the nightly run is where that would first be noticed.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type snapshotEnv struct {
	owner *pgx.Conn
	pool  *pgxpool.Pool
	store *Store
	// Both spellings of one workspace: the plain id the principal carries, and
	// the typed one the pool binder takes.
	ws      ids.UUID
	wsTyped ids.WorkspaceID
	rep     ids.UUID
}

func setupSnapshot(t *testing.T) *snapshotEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` " +
			"(integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	typed := ids.New[ids.WorkspaceKind]()
	e := &snapshotEnv{owner: owner, ws: typed.UUID, wsTyped: typed, rep: ids.NewV7()}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Manager')`,
		e.rep, "mgr-"+e.rep.String()+"@snapshot.test"); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	e.store = NewStore(database.BindTo(pool, e.wsTyped))
	return e
}

func (e *snapshotEnv) as() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"manager"},
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true, Create: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A population carrying every shape the writer has to build a row for: priced
// and converted, unpriced, and priced but unconvertible. Each pairs its columns
// differently, and each is a row the table can refuse.
func mixedPopulation(t *testing.T) Readings {
	t.Helper()
	priced := open(ids.NewV7().String(), 100_000)

	unpriced := open(ids.NewV7().String(), 0)
	unpriced.AmountMinor, unpriced.BaseMinor, unpriced.Currency = nil, nil, ""
	unpriced.WeightedMinor = 0
	unpriced.ExclusionReason = ExcludedUnpriced

	amount := int64(50_000)
	unconvertible := open(ids.NewV7().String(), 0)
	unconvertible.AmountMinor, unconvertible.Currency = &amount, "VND"
	unconvertible.BaseMinor = nil
	unconvertible.WeightedMinor = 0
	unconvertible.ExclusionReason = ExcludedFxMissing

	rows := []Contribution{priced, unpriced, unconvertible}
	return Readings{
		OpenMinor: 100_000, EvidenceMinor: 100_000, BestCaseMinor: 100_000,
		WeightedMinor: 50_000,
		EligibleCount: len(rows), PricedCount: 2,
		ConfirmedDateCount: len(rows), FxMissingCount: 1,
		Contributions: rows,
	}
}

func TestASnapshotWritesEveryContributionShapeTheTableAccepts(t *testing.T) {
	t.Parallel()
	e := setupSnapshot(t)
	ctx := e.as()

	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	// A different quarter from the arbiter test below, and deliberately so.
	// An installation holds exactly ONE workspace (database.go says the
	// workspace scopes nothing in SQL), so the daily arbiter is global to the
	// database — two tests sharing a period and a local day collide on it, and
	// that collision is the constraint working rather than a defect.
	period, err := ResolvePeriod(PeriodQuarter, time.Date(2026, time.February, 14, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatal(err)
	}

	var id ids.UUID
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = e.store.TakeSnapshot(ctx, tx, NewSnapshot{
			Period: period, Scope: Scope{Kind: ScopeWorkspace},
			Trigger: TriggerDaily, BaseCurrency: "EUR",
			Readings: mixedPopulation(t),
			TakenAt:  time.Date(2026, time.February, 14, 12, 0, 0, 0, zone),
		})
		return err
	}); err != nil {
		t.Fatalf("taking the snapshot: %v — a shape the writer builds is one the table refuses", err)
	}

	// Read it back through the real reader, so what is asserted is what a
	// movement diff would actually see.
	var back snapshotSide
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		back, err = e.store.SnapshotSide(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the snapshot back: %v", err)
	}
	if len(back.Contributions) != 3 {
		t.Fatalf("wrote 3 contributions and read %d back", len(back.Contributions))
	}

	// The identity, end to end: the stored rows sum to the stored headline.
	// This is the claim the whole module rests on, and it is the one that a
	// column mapped to the wrong position would break silently.
	var open int64
	for _, c := range back.Contributions {
		if c.InOpen && c.BaseMinor != nil {
			open += *c.BaseMinor
		}
	}
	if open != 100_000 {
		t.Errorf("the contributions read back sum to %d in open pipeline, want 100000 — "+
			"a headline that does not equal its own rows cannot be drilled into", open)
	}
}

// The daily arbiter, against the real index. Two daily snapshots for one
// period, scope and local day must not both stand: the dispatcher ticks
// repeatedly so a worker that was down still backfills, and without the
// constraint that produces two snapshots and no error.
func TestASecondDailySnapshotForTheSameDayIsRefused(t *testing.T) {
	t.Parallel()
	e := setupSnapshot(t)
	ctx := e.as()

	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	period, err := ResolvePeriod(PeriodQuarter, time.Date(2026, time.May, 14, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatal(err)
	}
	takenAt := time.Date(2026, time.May, 14, 9, 0, 0, 0, zone)

	take := func(trigger string, at time.Time) error {
		return e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := e.store.TakeSnapshot(ctx, tx, NewSnapshot{
				Period: period, Scope: Scope{Kind: ScopeWorkspace},
				Trigger: trigger, BaseCurrency: "EUR",
				Readings: Readings{Contributions: []Contribution{}},
				TakenAt:  at,
			})
			return err
		})
	}

	if err := take(TriggerDaily, takenAt); err != nil {
		t.Fatalf("the first daily snapshot was refused: %v", err)
	}
	// Later the same local day. The workspace scope's id is NULL, which a
	// partial unique index cannot arbitrate — the reason there are two indexes
	// rather than one, and the case that would otherwise be silently unguarded.
	if err := take(TriggerDaily, takenAt.Add(6*time.Hour)); err == nil {
		t.Error("a second DAILY snapshot for the same local day was accepted — the " +
			"dispatcher ticks repeatedly, so this produces two snapshots and no error")
	}
	// A call-triggered snapshot on the same day is deliberately additional: a
	// manager taking three calls in a day should get three frozen states.
	if err := take(TriggerCall, takenAt.Add(7*time.Hour)); err != nil {
		t.Errorf("a call snapshot on the same day was refused (%v) — only `daily` is "+
			"arbitrated, and a call is a real reason to freeze another state", err)
	}
}
