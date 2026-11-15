// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package reportingsubjects tests that one reporting evaluation reads each
// named team or owner once. The answers stay the same, and no subject or
// reader is admitted that was refused before.
package reportingsubjects

import (
	"context"
	"errors"
	"maps"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/snapshotreads"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// subjectReads counts the statements that name a subject, by their SQL shape.
type subjectReads struct {
	mu     sync.Mutex
	counts map[string]int
}

func (c *subjectReads) observe(sql string) {
	query := strings.Join(strings.Fields(sql), " ")
	kind := ""
	switch {
	case strings.HasPrefix(query, "SELECT display_name FROM app_user WHERE id = $1 AND "):
		kind = "owner"
	case query == "SELECT name FROM team WHERE id = $1 AND archived_at IS NULL":
		kind = "team"
	case strings.HasPrefix(query, "SELECT EXISTS ( SELECT 1 FROM team_membership tm "):
		kind = "membership"
	case query == "SELECT current_setting('transaction_isolation')":
		kind = "isolation"
	default:
		return
	}
	c.mu.Lock()
	c.counts[kind]++
	c.mu.Unlock()
}

func (c *subjectReads) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.observe(data.SQL)
	return ctx
}
func (*subjectReads) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *subjectReads) reset() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	was := c.counts
	c.counts = map[string]int{}
	return was
}

// world is the harness with a pool whose statements are counted. It holds an
// open deal owned by Rep1 in Team1 and a reader who may measure every owner.
type world struct {
	e      *integration.Env
	pool   *pgxpool.Pool
	reads  *subjectReads
	reader context.Context
	at     time.Time
}

func newWorld(t *testing.T) world {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Linden press", pipeline, open, &e.Rep1)
	e.WsExec(t, `UPDATE deal SET amount_minor = 250000, currency = 'EUR' WHERE id = $1`, deal)
	cfg, err := database.PoolConfig(os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns, cfg.MinConns = 2, 0
	reads := &subjectReads{counts: map[string]int{}}
	cfg.ConnConfig.Tracer = multitracer.New(cfg.ConnConfig.Tracer, reads)
	pool, err := testdb.OwnPoolFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"report_definition", "sales_target", "deal", "pipeline", "installation_settings", "forecast", "activity", "company", "contact", "team_oversight"} {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants})
	reader = auth.WithIndividualPerformance(integration.OnSeat(reader, principal.SeatFull), true)
	return world{e: e, pool: pool, reads: reads, reader: reader, at: clocktest.Now(t).UTC()}
}

func (w world) selection(kind string, id *ids.UUID) crmcontracts.ReportingSelection {
	scope := crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKind(kind)}
	if id != nil {
		wire := openapi_types.UUID(*id)
		scope.Id = &wire
	}
	return crmcontracts.ReportingSelection{
		Scope: scope, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate", "open_pipeline"},
		Blocks:  []crmcontracts.ReportingBlockKind{"bookings_trend", "stage_distribution"},
	}
}

type evaluated struct {
	result     crmcontracts.ReportingEvaluation
	statements int64
	reads      map[string]int
}

// evaluate runs one evaluation in a transaction of the given isolation and
// meters it. Read committed is the baseline: there every repeat reads again.
func (w world) evaluate(t *testing.T, level pgx.TxIsoLevel, selection crmcontracts.ReportingSelection) evaluated {
	t.Helper()
	service := compose.ReportingServiceFor(w.pool, func() time.Time { return w.at })
	metered, meter := database.WithStatementMeter(w.reader)
	var out evaluated
	w.reads.reset()
	if err := compose.InstallationDB(w.pool).TxIsolated(metered, level, func(tx pgx.Tx) error {
		before := meter.Cost()
		var err error
		out.result, err = service.EvaluateTx(metered, tx, selection)
		out.statements = meter.Cost().Minus(before).Statements
		return err
	}); err != nil {
		t.Fatal(err)
	}
	out.reads = w.reads.reset()
	return out
}

// A named team is resolved by the context, by every metric under its own
// savepoint and by the targets. In one snapshot it is read once. The answer,
// evaluation key included, equals that of an evaluation that reads it each time.
func TestANamedTeamIsReadOnceAcrossAnEvaluation(t *testing.T) {
	w := newWorld(t)
	for _, scope := range []struct {
		kind, read string
		id         ids.UUID
	}{{"team", "team", w.e.Team1}, {"owner", "owner", w.e.Rep1}} {
		selection := w.selection(scope.kind, &scope.id)
		uncached := w.evaluate(t, pgx.ReadCommitted, selection)
		cached := w.evaluate(t, pgx.RepeatableRead, selection)
		if uncached.reads[scope.read] < 3 {
			t.Fatalf("%s: the baseline read the subject %d times; the test needs repeats to measure", scope.kind, uncached.reads[scope.read])
		}
		if cached.reads[scope.read] != 1 || cached.reads["isolation"] != 1 {
			t.Fatalf("%s: one snapshot read the subject %d times and the isolation %d times; want 1 and 1", scope.kind, cached.reads[scope.read], cached.reads["isolation"])
		}
		if saved := uncached.statements - cached.statements; saved != int64(uncached.reads[scope.read]-1) {
			t.Fatalf("%s: statements %d uncached, %d cached; want %d fewer", scope.kind, uncached.statements, cached.statements, uncached.reads[scope.read]-1)
		}
		if !reflect.DeepEqual(uncached.result, cached.result) {
			t.Fatalf("%s: the answers differ:\nuncached %+v\ncached   %+v", scope.kind, uncached.result, cached.result)
		}
		if cached.result.Metrics[2].Value == nil || *cached.result.Metrics[2].Value == 0 {
			t.Fatalf("%s: the open deal is not in the pipeline: %+v", scope.kind, cached.result.Metrics[2])
		}
		t.Logf("%s scope: %d statements read committed, %d repeatable read; subject read %d times, then once",
			scope.kind, uncached.statements, cached.statements, uncached.reads[scope.read])
	}
	workspace := w.evaluate(t, pgx.RepeatableRead, w.selection("workspace", nil))
	if len(workspace.reads) != 0 {
		t.Fatalf("a workspace evaluation read subjects or the isolation: %v", workspace.reads)
	}
}

// Every evaluation starts with nothing remembered. A team archived or an owner
// deactivated after an earlier evaluation is refused, as is an unknown team.
func TestAnEvaluationRefusesSubjectsThatLeftSinceTheLastOne(t *testing.T) {
	w := newWorld(t)
	team, owner := w.selection("team", &w.e.Team2), w.selection("owner", &w.e.Rep3)
	for _, selection := range []crmcontracts.ReportingSelection{team, owner} {
		w.evaluate(t, pgx.RepeatableRead, selection)
	}
	users, admin, writer := identityWriter(t, w.e)
	archived := true
	if _, err := users.UpdateTeam(writer, admin, w.e.Team2, identity.UpdateTeamInput{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if err := users.DeactivateUser(writer, admin, identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](w.e.Rep3)}); err != nil {
		t.Fatal(err)
	}
	unknown := ids.NewV7()
	service := compose.ReportingServiceFor(w.pool, func() time.Time { return w.at })
	for _, selection := range []crmcontracts.ReportingSelection{team, owner, w.selection("team", &unknown)} {
		if _, err := service.Evaluate(w.reader, selection); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("the %s scope that left or never was evaluated: %v", selection.Scope.Kind, err)
		}
	}
}

// A snapshot remembers facts about subjects, never a decision about a reader.
// Each resolution still runs the reader's own rule. A membership is remembered
// only for the team set it was asked for.
func TestARememberedSubjectAdmitsNoOtherReader(t *testing.T) {
	w := newWorld(t)
	actor, _ := principal.Actor(w.reader)
	team := principal.Permissions{RowScope: principal.RowScopeTeam, Objects: actor.Permissions.Objects}
	own := principal.Permissions{RowScope: principal.RowScopeOwn, Objects: actor.Permissions.Objects}
	requested := compose.RequestedScope{Kind: compose.ScopeKindOwner, ID: &w.e.Rep3}
	as := func(ctx context.Context, perms principal.Permissions, teams ...ids.UUID) context.Context {
		a := actor
		a.Permissions, a.TeamIDs = perms, teams
		return principal.WithActor(ctx, a)
	}
	err := compose.InstallationDB(w.pool).TxIsolated(w.reader, pgx.RepeatableRead, func(tx pgx.Tx) error {
		ctx, release := snapshotreads.Open(w.reader, tx)
		defer release()
		w.reads.reset()
		for _, teams := range [][]ids.UUID{{w.e.Team2, w.e.Team1, w.e.Team2}, {w.e.Team1, w.e.Team2}} {
			if _, err := compose.ResolveAnalyticsScope(as(ctx, team, teams...), tx, requested); err != nil {
				return err
			}
		}
		if reads := w.reads.reset(); reads["membership"] != 1 || reads["owner"] != 1 {
			t.Fatalf("the same team set in another order read again: %v", reads)
		}
		if _, err := compose.ResolveAnalyticsScope(as(ctx, own, w.e.Team2), tx, requested); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("a reader limited to their own records measured a colleague: %v", err)
		}
		if _, err := compose.ResolveAnalyticsScope(as(ctx, team, w.e.Team1), tx, requested); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("a reader outside the colleague's team measured them: %v", err)
		}
		if _, err := compose.ResolveAnalyticsScope(auth.WithIndividualPerformance(as(ctx, team, w.e.Team2), false), tx, requested); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("a remembered subject passed the individual-performance switch: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func identityWriter(t *testing.T, e *integration.Env) (*identity.Service, identity.Identity, context.Context) {
	t.Helper()
	ctx := e.Admin()
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	for _, object := range []string{"team_admin", "user_admin"} {
		actor.Permissions.Objects[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	who := identity.Identity{
		UserID: ids.From[ids.UserKind](e.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		SeatType: string(principal.SeatFull), Roles: []string{"admin"}, Permissions: actor.Permissions,
	}
	return identity.NewServiceFor(compose.InstallationDB(e.Pool)), who, principal.WithActor(ctx, actor)
}
