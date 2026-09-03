// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assurance

// The scan's promises are about the DATABASE: an exception has one identity
// across nights, a resolved finding is not reopened by re-detection, and a run
// that could not read an upstream still exists to say so.
//
// None of those is visible to a unit test over this package's Go. The upsert's
// DO UPDATE clause is SQL, the uniqueness is an index, and "the run still
// exists" is a row.

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

type scanEnv struct {
	owner   *pgx.Conn
	pool    *pgxpool.Pool
	store   *Store
	ws      ids.UUID
	wsTyped ids.WorkspaceID
	rep     ids.UUID
}

func setupScan(t *testing.T) *scanEnv {
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
	e := &scanEnv{owner: owner, ws: typed.UUID, wsTyped: typed, rep: ids.NewV7()}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
		e.rep, "rep-"+e.rep.String()+"@assurance.test"); err != nil {
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

func (e *scanEnv) as() context.Context {
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

// A finding on two nights is ONE exception seen twice.
//
// Minted twice it would be two rows, and a manager would resolve the same thing
// every morning — which is what the logical key exists to prevent and what the
// unique index enforces.
func TestTheSameFindingOnTwoNightsIsOneException(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	ctx := e.as()

	dealID := ids.NewV7()
	finding := Finding{
		Type: TypeClosePast, SubjectID: dealID.String(), Severity: SeverityHigh,
		Claim:    map[string]any{"expected_close": "2026-04-30"},
		Observed: map[string]any{"as_of": "2026-05-14"},
	}

	for night := range 2 {
		// The second night observes a later date, which is what a real second
		// run would see — and exactly the thing the identity must not key on.
		if night == 1 {
			finding.Observed = map[string]any{"as_of": "2026-05-15"}
		}
		if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return e.store.UpsertException(ctx, tx, finding, e.rep.String())
		}); err != nil {
			t.Fatalf("night %d: recording the finding: %v", night, err)
		}
	}

	var rows int
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM assurance_exception WHERE subject_id = $1`,
			dealID).Scan(&rows)
	}); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("two nights produced %d exception rows, want 1 — a manager would "+
			"resolve the same finding every morning", rows)
	}
}

// A resolved exception is not reopened by re-detection.
//
// Somebody who answers "the value is correct" has said the condition will keep
// being true. Reopening it tonight asks them the same question every morning
// until they stop reading, which is how a review queue dies.
func TestAResolvedExceptionSurvivesTheNextScan(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	ctx := e.as()

	dealID := ids.NewV7()
	finding := Finding{
		Type: TypeBuyerSilent, SubjectID: dealID.String(), Severity: SeverityMedium,
		Claim:    map[string]any{"silent_days_threshold": 90},
		Observed: map[string]any{"silent_days": 91},
	}
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return e.store.UpsertException(ctx, tx, finding, e.rep.String())
	}); err != nil {
		t.Fatal(err)
	}

	// Somebody answers it.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE assurance_exception SET status = 'resolved' WHERE logical_key = $1`,
		LogicalKey(finding)); err != nil {
		t.Fatal(err)
	}

	// Tonight's scan finds the same condition, and a worse one.
	finding.Observed = map[string]any{"silent_days": 92}
	finding.Severity = SeverityHigh
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return e.store.UpsertException(ctx, tx, finding, e.rep.String())
	}); err != nil {
		t.Fatal(err)
	}

	var status, severity string
	var observed []byte
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT status, severity, observed FROM assurance_exception WHERE logical_key = $1`,
			LogicalKey(finding)).Scan(&status, &severity, &observed)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "resolved" {
		t.Errorf("a resolved exception came back as %q after the next scan — the person "+
			"who answered it would be asked again every morning", status)
	}
	// The value it was resolved AGAINST is kept, which is what lets a later
	// scan tell "still true" from "changed since you answered".
	if string(observed) == `{"silent_days": 92}` {
		t.Error("the resolved row's observed value was overwritten — nothing can then " +
			"tell whether the condition changed since it was answered")
	}
	if severity != SeverityMedium {
		t.Errorf("severity moved to %q on a resolved row", severity)
	}
}

// A run that could not read its inputs still exists, and says so.
//
// Refusing to start would produce no record in exactly the case this pass exists
// to report, and the brief waiting on it would run without ever learning why.
func TestARunWithUnreadableInputsStillExists(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	ctx := e.as()

	scanner := NewScanner(e.store,
		func(context.Context, pgx.Tx) ([]Subject, error) {
			return nil, context.DeadlineExceeded
		},
		func(context.Context, pgx.Tx, time.Time) []SourceCoverage {
			return []SourceCoverage{
				{Source: "mail", State: CoverageUnavailable},
				{Source: "offers", State: CoverageUnavailable},
			}
		},
		DefaultConfig())

	got, err := scanner.Scan(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("the scan refused to run: %v — a missing run is the one answer that "+
			"tells nobody anything", err)
	}
	if got.Status != StatusIncomplete {
		t.Errorf("status was %q, want %q", got.Status, StatusIncomplete)
	}
	if got.Readiness != ReadinessChecksIncomplete {
		t.Errorf("readiness was %q, want %q — a run that could not look must not grade "+
			"the pipeline", got.Readiness, ReadinessChecksIncomplete)
	}

	var status string
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT status FROM assurance_run WHERE id = $1`, got.RunID).Scan(&status)
	}); err != nil {
		t.Fatalf("the run row is not there: %v", err)
	}
	if status != StatusIncomplete {
		t.Errorf("the stored run says %q", status)
	}
}
