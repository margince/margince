// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package winchance_test

// A deal has one win chance, the number forecasting.ResolveWinChance gives.
// Nine readers show or weight it. They are the deal read, list, facts list and
// write reply, the worklist figures, the Home queue, the forecast, the report
// table and assurance. The probability filter and a saved list select by it. A
// quiet deal is halved while too few closed deals show what quiet does. After
// that the learned effect moves it. The accuracy view reads frozen forecasts.

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/winchance"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fixture is one installation with a pipeline and a deal store. The pipeline has
// a typed 20% first stage, a won stage and a lost stage. The test moves the
// store's clock.
type fixture struct {
	t        *testing.T
	env      *integration.Env
	owner    *pgx.Conn
	store    *deals.Store
	at       time.Time
	pipeline ids.UUID
	first    ids.UUID
	won      ids.UUID
	lost     ids.UUID
	writer   context.Context
	reader   context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	env := integration.Setup(t)
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set: run make db-up (integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	f := &fixture{t: t, env: env, owner: owner, at: clocktest.Now(t).Add(-30 * 24 * time.Hour)}
	f.store = deals.NewStore(env.DB(), compose.DealsInstallation()).SetClock(func() time.Time { return f.at })
	f.pipeline = f.seed(`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', false, 100)`)
	stage := func(name string, position int, semantic string, probability int) ids.UUID {
		return f.seed(`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, $3, $4, $5, $6)`,
			f.pipeline, name, position, semantic, probability)
	}
	// The deals' owner has a synced mailbox, so a silent customer reads as quiet.
	connection := f.seed(`INSERT INTO capture_connection (id, provider, user_id, status) VALUES ($1, 'gmail', $2, 'connected')`, env.Rep1)
	if _, err := owner.Exec(context.Background(), `INSERT INTO capture_sync_state (connection_id, last_success_at) VALUES ($1, now() - interval '3 hours')`, connection); err != nil {
		t.Fatalf("seeding the mailbox checkpoint: %v", err)
	}
	f.first, f.won, f.lost = stage("First", 0, "open", 20), stage("Won", 20, "won", 100), stage("Lost", 21, "lost", 0)
	if _, err := owner.Exec(context.Background(), `INSERT INTO setting (key, value) VALUES ('installation.base_currency', '"EUR"'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatalf("seeding the base currency: %v", err)
	}
	f.writer = principal.WithCorrelationID(principal.SystemActing(principal.WithWorkspaceID(context.Background(), env.WS), "system:winchance-test"), ids.NewV7())
	read := principal.ObjectGrant{Read: true}
	f.reader = principal.WithActor(principal.WithWorkspaceID(context.Background(), env.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + env.Rep1.String(), UserID: env.Rep1, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"deal": read, "forecast": read, "installation_settings": read, "pipeline": read,
		}},
	})
	return f
}

func (f *fixture) seed(sql string, args ...any) ids.UUID {
	f.t.Helper()
	id := ids.NewV7()
	if _, err := f.owner.Exec(context.Background(), sql, append([]any{id}, args...)...); err != nil {
		f.t.Fatalf("seeding: %v", err)
	}
	return id
}

// deal opens a deal in the first stage and moves it to each later stage, an
// hour after the last move.
func (f *fixture) deal(owner ids.UUID, amount int64, expected *time.Time, moves ...ids.UUID) ids.DealID {
	f.t.Helper()
	user, currency := ids.From[ids.UserKind](owner), "EUR"
	created, err := f.store.CreateDeal(f.writer, deals.CreateDealInput{
		Name: "Weighed deal", PipelineID: ids.From[ids.PipelineKind](f.pipeline), StageID: ids.From[ids.StageKind](f.first),
		OwnerID: &user, AmountMinor: &amount, Currency: &currency, ExpectedClose: expected, Source: "manual",
	})
	if err != nil {
		f.t.Fatal(err)
	}
	id := ids.From[ids.DealKind](ids.UUID(created.Id))
	for _, to := range moves {
		f.at = f.at.Add(time.Hour)
		f.close(id, to)
	}
	f.at = f.at.Add(time.Hour)
	return id
}

func (f *fixture) close(deal ids.DealID, to ids.UUID) {
	f.t.Helper()
	won, lost := "purchase_order", "price"
	input := deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](to)}
	switch to {
	case f.won:
		input.WonWithoutContractReason = &won
	case f.lost:
		input.LostReason = &lost
	}
	if _, err := f.store.AdvanceDeal(f.writer, deal, input); err != nil {
		f.t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
}

// quietDeal is an open deal due today that nobody has spoken to for 80 days.
func (f *fixture) quietDeal(today time.Time) ids.DealID {
	f.t.Helper()
	deal := f.deal(f.env.Rep1, 1_000_000, &today)
	quiet := today.AddDate(0, 0, -80)
	if _, err := f.owner.Exec(context.Background(), `UPDATE deal SET created_at = $2, last_activity_at = $2 WHERE id = $1`, deal.UUID, quiet); err != nil {
		f.t.Fatal(err)
	}
	return deal
}

func (f *fixture) inTx(fn func(ctx context.Context, tx pgx.Tx) error) {
	f.t.Helper()
	if err := database.WithWorkspaceTx(f.reader, f.env.Pool, func(tx pgx.Tx) error { return fn(f.reader, tx) }); err != nil {
		f.t.Fatal(err)
	}
}

// chances reads one deal's win chance through every surface.
func (f *fixture) chances(deal ids.DealID, today time.Time) map[string]int {
	f.t.Helper()
	out := map[string]int{}
	id := ids.From[ids.DealKind](deal.UUID)
	read, err := f.store.GetDeal(f.reader, id, storekit.LiveOnly)
	if err != nil {
		f.t.Fatal(err)
	}
	out["deal read"] = *read.WinProbability
	for name, in := range map[string]deals.ListDealsInput{"list": {}, "list facts": {FactsOnly: true}} {
		page, _, err := f.store.ListDeals(f.reader, in)
		if err != nil {
			f.t.Fatal(err)
		}
		for _, d := range page {
			if ids.UUID(d.Id) == deal.UUID {
				out[name] = *d.WinProbability
			}
		}
	}
	figures, err := f.store.Figures(f.reader, []ids.UUID{deal.UUID})
	if err != nil {
		f.t.Fatal(err)
	}
	out["figures"] = *figures[deal.UUID].WinProbability
	renamed := "Renamed in the write reply test"
	echoed, err := f.store.UpdateDeal(f.writer, id, deals.UpdateDealInput{Name: &renamed})
	if err != nil {
		f.t.Fatal(err)
	}
	out["write reply"] = *echoed.WinProbability
	engine := compose.WinChanceEngine()
	ranked, err := briefs.NewBriefEngine(f.env.Pool, f.env.Contacts).WithWinChances(engine.WinChances).Rank(f.reader, today)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, item := range ranked.Queue {
		if item.DealID == deal.UUID {
			out["home queue"] = int(math.Round(item.Features.Winnability * 100))
		}
	}
	f.inTx(func(ctx context.Context, tx pgx.Tx) error {
		table, err := engine.ReadTable(ctx, tx, today)
		if err != nil {
			return err
		}
		out["report table"] = table.Applied(deal.String(), f.first.String(), nil, 20)
		period, err := forecasting.ResolvePeriod(forecasting.PeriodQuarter, today, 1, time.UTC)
		if err != nil {
			return err
		}
		assembly, err := compose.ForecastAssemble(ctx, tx, period, forecasting.Scope{Kind: forecasting.ScopeWorkspace}, today, "EUR")
		if err != nil {
			return err
		}
		for _, row := range assembly.Readings.Contributions {
			if row.DealID == deal.String() {
				out["forecast"] = forecasting.AppliedProbability(row.StageProbability, row.Effect)
			}
		}
		subjects, err := compose.AssuranceSubjects(ctx, tx, today, compose.AssuranceCoverage(ctx, tx, today))
		for _, s := range subjects {
			if s.DealID == deal.String() {
				out["assurance"] = s.WinProbability
			}
		}
		return err
	})
	return out
}

func requireOneChance(t *testing.T, chances map[string]int, want int) {
	t.Helper()
	if len(chances) != 9 {
		t.Fatalf("read the chance through %d surfaces, want 9: %v", len(chances), chances)
	}
	for surface, got := range chances {
		if got != want {
			t.Errorf("%s reads %d%%, want %d%% like every other surface: %v", surface, got, want, chances)
		}
	}
}

// requireFilters checks that the probability filter and a saved list place the
// deal at the chance and no other.
func (f *fixture) requireFilters(deal ids.DealID, want int) {
	f.t.Helper()
	listed := func(low, high int) int {
		page, _, err := f.store.ListDeals(f.reader, deals.ListDealsInput{WinProbabilityMin: &low, WinProbabilityMax: &high})
		if err != nil {
			f.t.Fatal(err)
		}
		n := 0
		for _, d := range page {
			if ids.UUID(d.Id) == deal.UUID {
				n++
			}
		}
		return n
	}
	if listed(want, want) != 1 || listed(want+1, 100) != 0 || listed(0, want-1) != 0 {
		f.t.Errorf("the win probability filter does not place the deal at %d%%", want)
	}
	if f.savedListCount(want) != 1 || f.savedListCount(want+1) != 0 {
		f.t.Errorf("a saved list filtering on win_probability does not place the deal at %d%%", want)
	}
}

func (f *fixture) savedListCount(percent int) int {
	f.t.Helper()
	engine, ok, err := compose.NewCollectionsStore(f.env.Pool).SegmentEngine(f.reader, "deal")
	if err != nil || !ok {
		f.t.Fatalf("no deal segment engine: %v", err)
	}
	var count int
	f.inTx(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		count, err = engine.CountMatching(ctx, tx, storekit.Predicate{Field: "win_probability", Op: storekit.OpEq, Value: percent})
		return err
	})
	return count
}

func TestAQuietDealIsHalvedOnEverySurfaceWhileTheHistoryIsThin(t *testing.T) {
	f := newFixture(t)
	for range 20 {
		f.deal(f.env.Rep1, 0, nil, f.won)
	}
	today := clocktest.Now(t)
	f.at = today
	deal := f.quietDeal(today)

	// Twenty recorded wins lift the typed 20% to 60%. Quiet halves that to 30%.
	requireOneChance(t, f.chances(deal, today), 30)
	f.requireFilters(deal, 30)
	read, err := f.store.GetDeal(f.reader, ids.From[ids.DealKind](deal.UUID), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if chance := read.WinChance; chance == nil || chance.EffectLearned == nil || *chance.EffectLearned || chance.InUse != 60 {
		t.Fatalf("win chance %+v, want the halving fallback over a chance in use of 60", chance)
	}
}

func TestAQuietDealMovesByTheEffectLearnedFromSettledForecasts(t *testing.T) {
	f := newFixture(t)
	var quiet, calm []ids.DealID
	var rows []forecasting.Contribution
	for range forecasting.QuietSampleFloor {
		q, c := f.deal(f.env.Rep1, 0, nil), f.deal(f.env.Rep1, 0, nil)
		quiet, calm = append(quiet, q), append(calm, c)
		rows = append(rows,
			forecasting.Contribution{DealID: q.String(), InOpen: true, Signals: []forecasting.Signal{{Code: assurance.SignalQuiet}}},
			forecasting.Contribution{DealID: c.String(), InOpen: true, Signals: []forecasting.Signal{}})
	}
	f.freeze(f.at, f.at, rows, assurance.RiskSignalsVersion)
	for i := range quiet {
		f.close(quiet[i], f.lost)
		f.close(calm[i], f.won)
	}
	today := clocktest.Now(t)
	f.at = today
	deal := f.quietDeal(today)

	var evidence map[string]forecasting.StageEvidence
	var effects map[string]float64
	f.inTx(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if evidence, err = winchance.StageEvidence(ctx, tx, today); err != nil {
			return err
		}
		effects, err = winchance.RiskEffects(ctx, tx, today)
		return err
	})
	inUse := forecasting.ChanceInUse(20, evidence[f.first.String()])
	learned := effects[assurance.SignalQuiet]
	want := forecasting.AppliedProbability(inUse, &learned)
	if halved := inUse / 2; want == halved || learned >= 1 {
		t.Fatalf("learned effect %v gives %d%%, which cannot be told from the halving %d%%; the seed shows nothing", learned, want, halved)
	}

	requireOneChance(t, f.chances(deal, today), want)
	f.requireFilters(deal, want)
	read, err := f.store.GetDeal(f.reader, ids.From[ids.DealKind](deal.UUID), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if chance := read.WinChance; chance == nil || chance.EffectLearned == nil || !*chance.EffectLearned || *chance.EffectSample != forecasting.QuietSampleFloor {
		t.Fatalf("win chance %+v, want learned from %d settled deals", chance, forecasting.QuietSampleFloor)
	}
}

// At a Likely threshold of 50 the stored 60% would be Likely and the 30% win
// chance is not, so assurance and the forecast must read the chance.
func TestAssuranceReadsTheWinChanceForLikely(t *testing.T) {
	f := newFixture(t)
	for range 20 {
		f.deal(f.env.Rep1, 0, nil, f.won)
	}
	today := clocktest.Now(t)
	f.at = today
	deal := f.quietDeal(today)
	if _, err := f.owner.Exec(context.Background(), `INSERT INTO setting (key, value)
		VALUES ('installation.forecast_likely_threshold', '50'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	var subject assurance.Subject
	f.inTx(func(ctx context.Context, tx pgx.Tx) error {
		subjects, err := compose.AssuranceSubjects(ctx, tx, today, nil)
		for _, s := range subjects {
			if s.DealID == deal.String() {
				subject = s
			}
		}
		return err
	})
	if subject.WinProbability != 30 || subject.Likely {
		t.Fatalf("assurance sees %d%% (likely %v), want the win chance 30%% and not Likely", subject.WinProbability, subject.Likely)
	}
}

// freeze takes a daily workspace snapshot of the first stage's quarter at takenAt.
func (f *fixture) freeze(takenAt, within time.Time, rows []forecasting.Contribution, version string) {
	f.t.Helper()
	f.freezeQuarter(within, takenAt, forecasting.Readings{Contributions: rows}, version)
}

func (f *fixture) freezeQuarter(within, takenAt time.Time, readings forecasting.Readings, version string) {
	f.t.Helper()
	quarter, err := forecasting.ResolvePeriod(forecasting.PeriodQuarter, within, 1, time.UTC)
	if err != nil {
		f.t.Fatal(err)
	}
	store := forecasting.NewStore(f.env.DB())
	if err := store.InTx(f.writer, func(ctx context.Context, tx pgx.Tx) error {
		_, err := store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
			Period: quarter, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
			Trigger: forecasting.TriggerDaily, BaseCurrency: "EUR", TakenAt: takenAt, Readings: readings, RiskSignalsVersion: version,
		})
		return err
	}); err != nil {
		f.t.Fatalf("freezing the quarter: %v", err)
	}
	f.at = f.at.Add(time.Hour)
}

func (f *fixture) accuracy(at time.Time) (forecasting.Accuracy, error) {
	f.t.Helper()
	var out forecasting.Accuracy
	err := database.WithWorkspaceTx(f.reader, f.env.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = compose.WinChanceEngine().Accuracy(f.reader, tx, at)
		return err
	})
	return out, err
}

func (f *fixture) mustAccuracy(at time.Time) forecasting.Accuracy {
	f.t.Helper()
	out, err := f.accuracy(at)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fixture) showIndividualPerformance(on bool) {
	f.t.Helper()
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), f.env.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:settings-admin", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if err := config.Save(ctx, compose.StoredConfig(f.env.Pool), identity.IndividualPerformance, on); err != nil {
		f.t.Fatalf("switching individual performance to %v: %v", on, err)
	}
}

// The accuracy view sets each settled deal's frozen chance against how it
// closed. It reads the chance frozen at least two weeks before the close. A
// deal closed within two weeks of its freeze is not counted. That chance was
// read too late to be a forecast.
func TestTheAccuracyViewSetsFrozenChancesAgainstHowDealsClosed(t *testing.T) {
	f := newFixture(t)
	floor := analyticsquery.PercentileSampleFloor
	n := 2 * floor
	var settled []ids.DealID
	var rows []forecasting.Contribution
	for range n {
		deal := f.deal(f.env.Rep1, 0, nil)
		settled = append(settled, deal)
		rows = append(rows, forecasting.Contribution{DealID: deal.String(), InOpen: true, StageProbability: 64})
	}
	tooLate := f.deal(f.env.Rep1, 0, nil)
	f.at = f.at.Add(10 * 24 * time.Hour)
	rows = append(rows, forecasting.Contribution{DealID: tooLate.String(), InOpen: true, StageProbability: 64})
	f.freeze(f.at, f.at, rows, assurance.RiskSignalsVersion)
	f.at = f.at.Add(20 * 24 * time.Hour)
	for i, deal := range settled {
		if i < n/2 {
			f.close(deal, f.won)
		} else {
			f.close(deal, f.lost)
		}
	}
	today := f.at.Add(24 * time.Hour)
	f.at = f.at.Add(-10 * 24 * time.Hour)
	f.close(tooLate, f.lost)

	got := f.mustAccuracy(today).Calibration
	if got.Deals != n || len(got.Bands) != 1 {
		t.Fatalf("calibration holds %d deals in %d bands, want %d in one: %+v", got.Deals, len(got.Bands), n, got)
	}
	band := got.Bands[0]
	if band.From != 60 || band.To != 69 || band.MeanPercent == nil || *band.MeanPercent != 64 || band.WonPercent == nil || *band.WonPercent != 50 {
		t.Fatalf("band %+v, want 60 to 69 at a mean of 64 with half won", band)
	}
}

func day(month time.Month, d int) time.Time { return time.Date(2025, month, d, 12, 0, 0, 0, time.UTC) }

// A finished quarter is said to land at the won plus weighted pipeline of one
// freeze. That is the last freeze at least four weeks before the end. It is set
// against what the first freeze after the end shows as won.
func TestTheAccuracyViewSetsAQuartersFrozenLandingAgainstWhatWasWon(t *testing.T) {
	f := newFixture(t)
	within := day(time.May, 15)
	f.freezeQuarter(within, day(time.April, 2), forecasting.Readings{WonMinor: 0, WeightedMinor: 900}, "")
	f.freezeQuarter(within, day(time.May, 20), forecasting.Readings{WonMinor: 100, WeightedMinor: 400}, "")
	f.freezeQuarter(within, day(time.June, 20), forecasting.Readings{WonMinor: 300, WeightedMinor: 100}, "")
	f.freezeQuarter(within, day(time.July, 2), forecasting.Readings{WonMinor: 350}, "")

	landings := f.mustAccuracy(clocktest.Now(t)).Landings
	if len(landings) != 1 {
		t.Fatalf("%d landings, want the one finished quarter: %+v", len(landings), landings)
	}
	got := landings[0]
	if got.SaidMinor != 500 || got.LandedMinor != 350 || got.Currency != "EUR" || got.SaidOn.Day() != 20 {
		t.Fatalf("landing %+v, want said 500 on 20 May and landed 350", got)
	}
}

// The landings are the workspace's frozen totals. GET /forecast shows those
// only to a seat that may assert about the workspace. A seat reading only its
// own or its team's rows may not. A mask never reaches a seat reading every
// row, so a masked seat is one of these too.
func TestTheAccuracyViewShowsLandingsOnlyToASeatThatMaySeeTheFrozenWorkspace(t *testing.T) {
	f := newFixture(t)
	within := day(time.May, 15)
	f.freezeQuarter(within, day(time.May, 20), forecasting.Readings{WonMinor: 100, WeightedMinor: 400}, "")
	f.freezeQuarter(within, day(time.July, 2), forecasting.Readings{WonMinor: 350}, "")
	if got := f.mustAccuracy(clocktest.Now(t)).Landings; len(got) != 1 {
		t.Fatalf("a seat reading every row got %d landings, want the one quarter", len(got))
	}
	allRows := f.reader
	for _, scope := range []principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam} {
		p, ok := principal.Actor(allRows)
		if !ok {
			t.Fatal("the fixture reader carries no principal")
		}
		p.Permissions.RowScope = scope
		f.reader = principal.WithActor(allRows, p)
		if got := f.mustAccuracy(clocktest.Now(t)).Landings; len(got) != 0 {
			t.Errorf("a seat with row scope %q got %d workspace landings, want none: %+v", scope, len(got), got)
		}
	}
}

// With individual performance off, a landing that one colleague's deals make
// up is withheld. The totals would give that colleague's pipeline away.
func TestTheAccuracyViewWithholdsALandingOfOneColleague(t *testing.T) {
	f := newFixture(t)
	deal := f.deal(f.env.Rep3, 0, nil)
	owned := []forecasting.Contribution{{DealID: deal.String(), Owner: f.env.Rep3.String(), InOpen: true}}
	within := day(time.May, 15)
	f.freezeQuarter(within, day(time.May, 20), forecasting.Readings{WonMinor: 100, WeightedMinor: 400, Contributions: owned}, "")
	f.freezeQuarter(within, day(time.July, 2), forecasting.Readings{WonMinor: 350, Contributions: owned}, "")

	f.showIndividualPerformance(true)
	if got := f.mustAccuracy(clocktest.Now(t)).Landings; len(got) != 1 {
		t.Fatalf("with individual performance on, %d landings, want the one quarter", len(got))
	}
	f.showIndividualPerformance(false)
	_, err := f.accuracy(clocktest.Now(t))
	if _, ok := errors.AsType[auth.IndividualPerformanceOffError](err); !ok {
		t.Fatalf("a landing of one colleague answered %v, want the individual performance refusal", err)
	}
}

// A quarter is finished when it ended on the installation's own calendar. At
// 18:00 UTC on its last day it is already over in Tokyo and not yet in UTC.
func TestTheAccuracyViewJudgesAFinishedQuarterByTheInstallationsDay(t *testing.T) {
	f := newFixture(t)
	within := day(time.May, 15)
	f.freezeQuarter(within, day(time.May, 20), forecasting.Readings{WonMinor: 100, WeightedMinor: 400}, "")
	f.freezeQuarter(within, time.Date(2025, time.July, 1, 2, 0, 0, 0, time.UTC), forecasting.Readings{WonMinor: 350}, "")
	lastEvening := time.Date(2025, time.June, 30, 18, 0, 0, 0, time.UTC)
	setZone := func(zone string) {
		t.Helper()
		if _, err := f.owner.Exec(context.Background(), `INSERT INTO setting (key, value) VALUES ('installation.timezone', to_jsonb($1::text))
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, zone); err != nil {
			t.Fatal(err)
		}
	}
	setZone("UTC")
	if got := f.mustAccuracy(lastEvening).Landings; len(got) != 0 {
		t.Fatalf("in UTC the quarter has not ended yet, but %d landings were read", len(got))
	}
	setZone("Asia/Tokyo")
	if got := f.mustAccuracy(lastEvening).Landings; len(got) != 1 {
		t.Fatalf("in Tokyo the quarter ended, but %d landings were read", len(got))
	}
}
