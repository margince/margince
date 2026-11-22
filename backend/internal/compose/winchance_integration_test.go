// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A deal has one win chance, the number forecasting.ResolveWinChance gives.
// Six surfaces read it. They are the forecast, the tool, the deal read, the
// deal list filter, the worklist figures and the forecast report. A quiet deal
// is halved while too few closed deals show what quiet does. After that the
// learned effect moves it.

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// quietOpenDeal is an open deal in the first stage, due today, that nobody has
// spoken to for 80 days.
func (f *stageCohortFixture) quietOpenDeal(t *testing.T, today time.Time) ids.UUID {
	t.Helper()
	deal := f.openDeal(t, f.stage(0), today)
	quiet := today.AddDate(0, 0, -80)
	if _, err := f.env.owner.Exec(context.Background(),
		`UPDATE deal SET created_at = $2, last_activity_at = $2 WHERE id = $1`, deal, quiet); err != nil {
		t.Fatal(err)
	}
	return deal
}

// surfaceChances reads one deal's win chance through every surface that shows
// or weights it.
func (f *stageCohortFixture) surfaceChances(t *testing.T, deal ids.UUID, today time.Time) map[string]int {
	t.Helper()
	reader := f.forecastReader()
	out := map[string]int{}
	out["forecast"] = contributionOfDeal(t, *f.forecastOverHTTP(t, today).Contributions, deal).AppliedProbability
	for _, row := range f.forecastOverTool(t, today).Contributions {
		if row.DealID == deal.String() {
			out["forecast tool"] = row.AppliedProbability
		}
	}
	read, err := f.store.GetDeal(reader, ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	out["deal read"] = *read.WinProbability
	figures, err := f.store.Figures(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatal(err)
	}
	out["figures"] = *figures[deal].WinProbability
	ranked, err := briefs.NewBriefEngine(f.env.Pool, contacts.NewStore(InstallationDB(f.env.Pool))).WithWinChances(dealWinChances).Rank(reader, today)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range ranked.Queue {
		if item.DealID == deal {
			out["home queue"] = int(math.Round(item.Features.Winnability * 100))
		}
	}
	return out
}

func requireOneChance(t *testing.T, chances map[string]int, want int) {
	t.Helper()
	if len(chances) != 5 {
		t.Fatalf("read the chance through %d surfaces, want 5: %v", len(chances), chances)
	}
	for surface, got := range chances {
		if got != want {
			t.Errorf("%s reads %d%%, want %d%% like every other surface: %v", surface, got, want, chances)
		}
	}
}

// The deal list filter and the forecast report read the same number too.
func (f *stageCohortFixture) requireFilterAndReportAt(t *testing.T, deal ids.UUID, want int) {
	t.Helper()
	listed := func(min, max int) int {
		page, _, err := f.store.ListDeals(f.forecastReader(), deals.ListDealsInput{
			RangeFilters: deals.RangeFilters{WinProbabilityMin: &min, WinProbabilityMax: &max},
		})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, d := range page {
			if ids.UUID(d.Id) == deal {
				n++
			}
		}
		return n
	}
	if listed(want, want) != 1 || listed(want+1, 100) != 0 || listed(0, want-1) != 0 {
		t.Errorf("the win probability filter does not place the deal at exactly %d%%", want)
	}
	result := f.env.runReport(f.env.Admin(), t, "forecast", `{"group_by":["currency"]}`)
	for _, row := range result.Rows {
		if row["currency"] == "EUR" {
			if got, wantMinor := wireInt(t, row, "weighted_minor"), int64(1_000_000*want/100); got != wantMinor {
				t.Errorf("the forecast report weights the deal at %d, want %d (%d%% of 1000000)", got, wantMinor, want)
			}
			return
		}
	}
	t.Fatalf("no EUR row in the forecast report: %+v", result.Rows)
}

func TestAQuietDealIsHalvedOnEverySurfaceWhileTheHistoryIsThin(t *testing.T) {
	f := newStageCohortFixture(t)
	for range 20 {
		f.deal(t, f.env.Rep1, f.stage(0), f.won)
	}
	today := clocktest.Now(t)
	f.at = today
	deal := f.quietOpenDeal(t, today)

	// Twenty recorded wins lift the typed 20% to 60%; quiet halves that to 30%.
	requireOneChance(t, f.surfaceChances(t, deal, today), 30)
	f.requireFilterAndReportAt(t, deal, 30)

	read, err := f.store.GetDeal(f.forecastReader(), ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if chance := read.WinChance; chance == nil || chance.EffectLearned == nil || *chance.EffectLearned || chance.InUse != 60 {
		t.Fatalf("win chance %+v, want the halving fallback over a chance in use of 60", chance)
	}
}

func TestAQuietDealMovesByTheEffectLearnedFromSettledForecasts(t *testing.T) {
	f := newStageCohortFixture(t)
	s0 := f.stage(0)
	var quiet, calm []ids.DealID
	var rows []forecasting.Contribution
	for range forecasting.QuietSampleFloor {
		q, c := f.deal(t, f.env.Rep1, s0), f.deal(t, f.env.Rep1, s0)
		quiet, calm = append(quiet, q), append(calm, c)
		rows = append(rows,
			forecasting.Contribution{DealID: q.String(), InOpen: true, Signals: []forecasting.Signal{{Code: assurance.SignalQuiet}}},
			forecasting.Contribution{DealID: c.String(), InOpen: true, Signals: []forecasting.Signal{}})
	}
	f.freezeSignals(t, rows, forecasting.TriggerDaily, assurance.RiskSignalsVersion)
	for i := range quiet {
		f.lose(t, quiet[i])
		f.win(t, calm[i])
	}
	today := clocktest.Now(t)
	f.at = today
	deal := f.quietOpenDeal(t, today)

	var evidence map[string]forecasting.StageEvidence
	var effects map[string]float64
	if err := database.WithWorkspaceTx(f.writer, f.env.Pool, func(tx pgx.Tx) error {
		var err error
		if evidence, err = forecastStageEvidence(f.writer, tx, today); err != nil {
			return err
		}
		effects, err = forecastRiskEffects(f.writer, tx, today)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	inUse := forecasting.ChanceInUse(20, evidence[s0.String()])
	learned := effects[assurance.SignalQuiet]
	want := forecasting.AppliedProbability(inUse, &learned)
	if halved := inUse / 2; want == halved || learned >= 1 {
		t.Fatalf("learned effect %v gives %d%%, which cannot be told from the halving %d%%; the seed shows nothing", learned, want, halved)
	}

	requireOneChance(t, f.surfaceChances(t, deal, today), want)
	f.requireFilterAndReportAt(t, deal, want)
	read, err := f.store.GetDeal(f.forecastReader(), ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if chance := read.WinChance; chance == nil || chance.EffectLearned == nil || !*chance.EffectLearned || *chance.EffectSample != forecasting.QuietSampleFloor {
		t.Fatalf("win chance %+v, want learned from %d settled deals", chance, forecasting.QuietSampleFloor)
	}
}
