// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// worklistStages are the stages a worklist read times on a day with an open
// task. A stage that loses its timer fails this list, and its statements move
// to outside_stages, whose count is fixed below.
var worklistStages = []string{
	"this_morning", "needs_you", "planned", "relationship_decay", "subject_labels",
	"waiting_customers", "plan", "awaiting", "deal_moves",
	"meeting_horizon", "rank", "row_names",
}

// worklistOutsideStatements counts BEGIN, COMMIT, the reads before the day is
// assembled, and the snapshot's plan-mode setup and restore. A lane's deferred
// savepoint statements count where the exchange that carries them does.
const worklistOutsideStatements = 9

// The seeded read includes every bound lane and three open tasks, which are
// undated and so drawn under Upcoming. Lower this pgx-call ceiling when a
// batch or empty-set shortcut removes calls.
// waiting_customers reads the private_mail_requests setting once a transaction.
// needs_you reads the reader's duplicate pairs and their count in one statement.
const worklistExchangeBudget = 75

const worklistRoundTripBudget = worklistExchangeBudget

// stageDelta keeps stage totals separate from the pool meter.
type stageDelta struct {
	statements, exchanges, clipped int64
	runs                           map[attention.StageKey]uint64
	byKey                          map[attention.StageKey]int64
	exchangesBy                    map[attention.StageKey]int64
}

// meterRead keeps both meters on the same read, so statements
// outside the stages remain visible.
func meterRead(ctx context.Context, t *testing.T, read func(context.Context) error) (stageDelta, database.StatementCost) {
	t.Helper()
	ctx, meter := database.WithStatementMeter(ctx)
	before := attention.StageCosts()
	if err := read(ctx); err != nil {
		t.Fatal(err)
	}
	after := attention.StageCosts()
	delta := stageDelta{
		runs: map[attention.StageKey]uint64{}, byKey: map[attention.StageKey]int64{}, exchangesBy: map[attention.StageKey]int64{},
	}
	for key, cost := range after {
		runs := cost.DB.Count() - before[key].DB.Count()
		if runs == 0 {
			continue
		}
		delta.runs[key] = runs
		delta.byKey[key] = cost.Statements - before[key].Statements
		delta.statements += delta.byKey[key]
		delta.exchangesBy[key] = cost.Exchanges - before[key].Exchanges
		delta.exchanges += delta.exchangesBy[key]
		delta.clipped += cost.Clipped - before[key].Clipped
		t.Logf("%s %-28s %3d statements, %3d exchanges, %6.2f ms pgx", key.Read, key.Stage, delta.byKey[key],
			cost.Exchanges-before[key].Exchanges,
			(cost.DB.Sum()-before[key].DB.Sum())*1000)
	}
	return delta, meter.Cost()
}

// seedOpenTask writes three open tasks, so a read made once per task costs
// more than the budget allows.
func seedOpenTask(t *testing.T, e *Env) {
	t.Helper()
	pipeline, stage, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Renewal", pipeline, stage, nil)
	for _, subject := range []string{"Send the renewal terms", "Call the buyer", "Book the review"} {
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: &subject, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// meteredAttention is the attention service on a pool of its own that counts
// protocol round trips. Idle pings and retirement are off, so only request
// work is measured.
func meteredAttention(t *testing.T, e *Env, at time.Time) (*attention.Service, *atomic.Int64) {
	t.Helper()
	cfg := e.Pool.Config()
	cfg.ShouldPing = func(context.Context, pgxpool.ShouldPingParams) bool { return false }
	cfg.MaxConnLifetime = 0
	cfg.MaxConnIdleTime = time.Hour
	cfg.HealthCheckPeriod = time.Hour
	// Framing is measured only on this isolated local test connection.
	cfg.ConnConfig.TLSConfig = nil
	cfg.ConnConfig.Fallbacks = nil
	trips := testdb.CountRoundTrips(t, cfg)
	pool, err := testdb.OwnPoolFromConfig(e.Admin(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return compose.NewAttentionService(pool, approvals.NewService(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.WS))), func() time.Time { return at }), trips
}

// The stages of one worklist read account for every statement the pool's
// tracer saw for it, and the read opens its snapshot once.
func TestAWorklistReadsStatementsAddUpToWhatThePoolSaw(t *testing.T) {
	e := Setup(t)
	seedOpenTask(t, e)
	service, trips := meteredAttention(t, e, clocktest.Now(t).UTC())
	read := func(ctx context.Context) error {
		_, err := service.Worklist(ctx, "mine", "all", ids.UUID{}, 0, "")
		return err
	}
	// The first read fills the stores' cached workspace, which a running
	// server has long since done.
	if err := read(e.Admin()); err != nil {
		t.Fatal(err)
	}

	beforeTrips := trips.Load()
	delta, seen := meterRead(e.Admin(), t, read)
	roundTrips := trips.Load() - beforeTrips
	t.Logf("worklist protocol round trips=%d", roundTrips)
	if roundTrips < seen.Exchanges || roundTrips > worklistRoundTripBudget {
		t.Errorf("worklist used %d protocol round trips for %d pgx calls, budget %d", roundTrips, seen.Exchanges, worklistRoundTripBudget)
	}
	t.Logf("worklist statements=%d exchanges=%d prepares=%d transactions=%d", seen.Statements, seen.Exchanges, seen.Prepares, seen.Transactions)

	if seen.Statements == 0 || delta.statements != seen.Statements {
		t.Fatalf("the stages counted %d statements, the pool's tracer saw %d; every statement belongs to one stage",
			delta.statements, seen.Statements)
	}
	if seen.Exchanges == 0 || delta.exchanges != seen.Exchanges {
		t.Fatalf("the stages counted %d exchanges, the pool's tracer saw %d", delta.exchanges, seen.Exchanges)
	}
	if seen.Exchanges > worklistExchangeBudget {
		t.Errorf("worklist used %d database exchanges, budget %d", seen.Exchanges, worklistExchangeBudget)
	}
	for key := range delta.runs {
		if key.Read != attention.ReadWorklist {
			t.Errorf("a worklist read charged %s/%s", key.Read, key.Stage)
		}
	}
	for _, stage := range worklistStages {
		if delta.runs[attention.StageKey{Read: attention.ReadWorklist, Stage: stage}] != 1 {
			t.Errorf("stage %s was not metered once by the read", stage)
		}
	}
	if delta.runs[attention.StageKey{Read: attention.ReadWorklist, Stage: "done_for_you"}] != 0 {
		t.Error("the read assembled the receipt lane, which is never a queue row")
	}
	outside := delta.byKey[attention.StageKey{Read: attention.ReadWorklist, Stage: attention.OutsideStages}]
	if outside != worklistOutsideStatements {
		t.Errorf("%d statements ran outside every stage, want %d; a stage that lost its timer moves its statements here",
			outside, worklistOutsideStatements)
	}
	// The snapshot, and the walk freeze, which commits on its own connection.
	if seen.Acquires != 2 || seen.PoolWait <= 0 {
		t.Errorf("the read acquired %d connections waiting %v, want the snapshot and the walk freeze", seen.Acquires, seen.PoolWait)
	}
	if delta.clipped != 0 {
		t.Errorf("%d stage runs took less time than their pgx calls; a read's calls run one after another", delta.clipped)
	}
}

// Home's read, as the browser asks for it, within its statement and round-trip
// ceilings. Lower them in the diff that removes statements or exchanges. The
// read is the assembly a cache miss pays; the cache's own lookup and store are
// outside it.
// Both count waiting_customers' private_mail_requests read, the undated tasks'
// planned, row_names and subject_labels reads, and needs_you's duplicate pairs.
const (
	homeReadStatementCeiling = 102
	homeReadRoundTripCeiling = 69
)

// homeStages are the stages Home's read runs. A lane that stops running lowers
// both counts, so the ceilings alone would pass it; this list fails it.
var homeStages = []string{
	"ai_work_health", "at_risk", "automation_health", "awaiting", "booking_health", "bounces",
	"capture_health", "deal_moves", "did_not_run", "domain_questions", "dsr", "introductions",
	"meeting_horizon", "meetings", "meetings_unreported", "needs_you", "notice_case", "notices",
	attention.OutsideStages, "plan", "planned", "rank", "row_names", "subject_labels",
	"this_morning", "undelivered", "waiting_customers",
}

func TestHomesReadStaysWithinItsStatementAndRoundTripBudgets(t *testing.T) {
	e := Setup(t)
	seedOpenTask(t, e)
	service, trips := meteredAttention(t, e, clocktest.Now(t).UTC())
	home := service.OmittingRelationships().ShapedForHome()
	read := func(ctx context.Context) error {
		_, err := home.WorklistView(ctx, "today", "mine", "all", ids.UUID{}, 100, "")
		return err
	}
	if err := read(e.Admin()); err != nil {
		t.Fatal(err)
	}

	beforeTrips := trips.Load()
	delta, seen := meterRead(e.Admin(), t, read)
	roundTrips := trips.Load() - beforeTrips
	t.Logf("home statements=%d exchanges=%d round trips=%d", seen.Statements, seen.Exchanges, roundTrips)
	if seen.Statements == 0 || delta.statements != seen.Statements {
		t.Fatalf("the stages counted %d statements, the pool's tracer saw %d", delta.statements, seen.Statements)
	}
	if seen.Statements > homeReadStatementCeiling {
		t.Errorf("Home's read sent %d statements, budget %d", seen.Statements, homeReadStatementCeiling)
	}
	if roundTrips < seen.Exchanges || roundTrips > homeReadRoundTripCeiling {
		t.Errorf("Home's read used %d protocol round trips for %d pgx calls, budget %d", roundTrips, seen.Exchanges, homeReadRoundTripCeiling)
	}
	ran := map[string]bool{}
	for key := range delta.runs {
		ran[key.Stage] = true
	}
	for _, stage := range homeStages {
		if !ran[stage] {
			t.Errorf("Home's read ran no %s stage; a lane that stops running would pass the ceilings", stage)
		}
		delete(ran, stage)
	}
	for stage := range ran {
		t.Errorf("Home's read ran stage %s, which homeStages does not name", stage)
	}
}

// Team exceptions reads the assembled day inside its own snapshot. Its stages
// are charged to it once, never to a read nested in it.
func TestTeamExceptionsChargesTheDayItReadsToItselfOnce(t *testing.T) {
	e := Setup(t)
	seedOpenTask(t, e)
	at := clocktest.Now(t).UTC()
	service := compose.NewAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })

	delta, seen := meterRead(e.Admin(), t, func(ctx context.Context) error {
		_, err := service.TeamExceptions(ctx)
		return err
	})

	if seen.Statements == 0 || delta.statements != seen.Statements {
		t.Fatalf("the stages counted %d statements, the pool's tracer saw %d", delta.statements, seen.Statements)
	}
	for key, runs := range delta.runs {
		if key.Read != attention.ReadTeamExceptions || runs != 1 {
			t.Errorf("%s/%s ran %d times, want each stage once under %s",
				key.Read, key.Stage, runs, attention.ReadTeamExceptions)
		}
	}
	t.Logf("team exceptions acquired %d pool connections", seen.Acquires)
}

// cutWaitingStageExchangeBudget caps the waiting_customers stage when the scan
// is cut. It covers the scan, its email rows, and one read of the deals past it.
const cutWaitingStageExchangeBudget = 18

// cutScan is Rep1's waiting scan cut by plain mail, with two at-risk deals
// filed under no mail yet.
type cutScan struct {
	e        *Env
	at       time.Time
	contact  ids.UUID
	rep1     context.Context
	waitedOn ids.UUID
	second   ids.UUID
}

func seedCutScan(t *testing.T, e *Env, at time.Time) cutScan {
	t.Helper()
	c := cutScan{
		e: e, at: at, contact: e.SeedContact(t, "Dana Buyer", nil),
		rep1: e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms),
	}
	first := c.mail(t, "Question 0", at.Add(-time.Minute), nil)
	e.CloneActivity(t, first, activities.WaitingScanCap+5, map[string]string{
		"source_id":    "r.source_id || '-' || n.ord",
		"identity_key": "r.identity_key || '-' || n.ord",
		"subject":      "'Question ' || n.ord",
		"thread_key":   "'question-' || n.ord",
		"occurred_at":  "r.occurred_at - n.ord * interval '1 minute'",
	})
	pipeline, stage, _ := DealFixture(t, e)
	for _, deal := range []*ids.UUID{&c.waitedOn, &c.second} {
		*deal = e.SeedDeal(t, "Past its close", pipeline, stage, &e.Rep1)
		e.WsExec(t, `UPDATE deal SET expected_close_date = $2 WHERE id = $1`, *deal, at.AddDate(0, 0, -5).Format(time.DateOnly))
	}
	c.mail(t, "About the first deal", at.AddDate(0, 0, -2), &c.waitedOn)
	return c
}

// mail logs one inbound mail to Rep1 from the contact, filed under deal when
// one is named.
func (c cutScan) mail(t *testing.T, subject string, at time.Time, deal *ids.UUID) ids.ActivityID {
	t.Helper()
	inbound := "inbound"
	to := c.e.WsScalar(t, `SELECT lower(email) FROM app_user WHERE id = $1`, c.e.Rep1)
	links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: c.contact}}
	if deal != nil {
		links = append(links, activities.ActivityLinkInput{EntityType: "deal", EntityID: *deal})
	}
	row, _, err := c.e.Activities.LogActivity(c.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
		ThreadKey: "thread-" + subject, CounterpartyEmail: "dana@buyer.example", EmailFrom: "dana@buyer.example",
		EmailTo: []string{to}, Links: links,
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(row.Id))
}

// A read whose waiting scan was cut asks once which at-risk deals wait past
// it. The waiting stage stays inside its budget, and a second deal waited on
// costs nothing more than the first.
func TestACutWaitingScanAsksForTheDealsPastItOnce(t *testing.T) {
	e := Setup(t)
	at := clocktest.Now(t).UTC()
	c := seedCutScan(t, e, at)
	service, _ := meteredAttention(t, e, at)
	stage := attention.StageKey{Read: attention.ReadWorklist, Stage: "waiting_customers"}
	read := func() (int64, int64, map[string]bool) {
		t.Helper()
		var page crmcontracts.Worklist
		delta, _ := meterRead(c.rep1, t, func(ctx context.Context) error {
			var err error
			page, err = service.Worklist(ctx, "mine", "all", ids.UUID{}, 0, "")
			return err
		})
		atRisk := map[string]bool{}
		for _, row := range page.Queue {
			if row.Source == crmcontracts.WorklistItemSourceDealAtRisk {
				atRisk[row.Id] = true
			}
		}
		return delta.byKey[stage], delta.exchangesBy[stage], atRisk
	}
	read()

	oneStatements, oneExchanges, atRisk := read()
	if atRisk[c.waitedOn.String()] || !atRisk[c.second.String()] {
		t.Fatalf("at-risk rows %v: want the second deal and not the one waited on past the scan", atRisk)
	}
	if oneExchanges > cutWaitingStageExchangeBudget {
		t.Errorf("the cut waiting stage used %d exchanges, budget %d", oneExchanges, cutWaitingStageExchangeBudget)
	}
	c.mail(t, "About the second deal", at.AddDate(0, 0, -2), &c.second)
	twoStatements, twoExchanges, atRisk := read()
	if atRisk[c.second.String()] {
		t.Error("the second deal waited on past the scan is still shown at risk")
	}
	if twoStatements != oneStatements || twoExchanges != oneExchanges {
		t.Errorf("two deals waited on cost %d statements and %d exchanges, one cost %d and %d",
			twoStatements, twoExchanges, oneStatements, oneExchanges)
	}
}
