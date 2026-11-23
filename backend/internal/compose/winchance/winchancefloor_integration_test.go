// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package winchance_test

// While "Show individual performance" is off, a stage's recorded outcomes count
// toward a deal's chance only when the floor holds them back from no reader.
// Every surface then reads the same chance, and the stage chances agree.

import (
	"context"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// liveSeat adds a live member on no team.
func (f *fixture) liveSeat(name string) ids.UUID {
	f.t.Helper()
	return f.seed(`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, $3)`, name+"@floor.test", name)
}

// wonFrom closes n deals won through the first stage, owned by owners in turn.
// They close a year ago, so no forecast of this quarter counts them as won.
func (f *fixture) wonFrom(n int, owners ...ids.UUID) {
	f.t.Helper()
	f.at = clocktest.Now(f.t).AddDate(-1, 0, 0)
	for i := range n {
		deal := f.deal(f.env.Rep1, 0, nil, f.won)
		if _, err := f.owner.Exec(context.Background(), `UPDATE deal SET owner_id = $2 WHERE id = $1`, deal.UUID, owners[i%len(owners)]); err != nil {
			f.t.Fatalf("handing the deal on: %v", err)
		}
	}
}

// firstStageEvidence is how many closed entries the stage chances show for
// the first stage.
func (f *fixture) firstStageEvidence() int {
	f.t.Helper()
	var stages forecasting.StageChances
	f.inTx(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		stages, err = compose.WinChanceEngine().StageChances(ctx, tx, clocktest.Now(f.t))
		return err
	})
	for _, stage := range stages.Stages {
		if stage.StageID.String() == f.first.String() {
			return stage.Evidence.Resolved
		}
	}
	f.t.Fatal("the stage chances leave out the first stage")
	return 0
}

// readerEvidence is the closed entries and won share the reader's deal read
// carries.
func (f *fixture) readerEvidence(deal ids.DealID) (int, int) {
	f.t.Helper()
	read, err := f.store.GetDeal(f.reader, ids.From[ids.DealKind](deal.UUID), storekit.LiveOnly)
	if err != nil {
		f.t.Fatal(err)
	}
	chance := read.WinChance
	if chance == nil || chance.EvidenceN == nil || chance.WonShare == nil {
		f.t.Fatalf("win chance %+v carries no evidence fields", chance)
	}
	return *chance.EvidenceN, *chance.WonShare
}

func TestALoneColleaguesStageOutcomesMoveNoReadersChanceWhileTheSwitchIsOff(t *testing.T) {
	f := newFixture(t)
	f.wonFrom(20, f.env.Rep2)
	today := clocktest.Now(t)
	f.at = today
	deal := f.deal(f.env.Rep1, 1_000_000, &today)

	// Twenty of the colleague's wins lift the typed 20% to 60%.
	requireOneChance(t, f.chances(deal, today), 60)
	if f.firstStageEvidence() != 20 {
		t.Fatal("the stage chances do not show the colleague's twenty wins while the switch is on")
	}

	f.showIndividualPerformance(false)
	requireOneChance(t, f.chances(deal, today), 20)
	f.requireFilters(deal, 20)
	if n, share := f.readerEvidence(deal); n != 0 || share != 0 {
		t.Errorf("the deal read shows %d closed entries and %d%% won, want neither from one colleague's deals", n, share)
	}
	if n := f.firstStageEvidence(); n != 0 {
		t.Errorf("the stage chances show %d closed entries, want none from one colleague's deals", n)
	}
}

func TestStageOutcomesOfEnoughColleaguesStillMoveTheChanceWhileTheSwitchIsOff(t *testing.T) {
	f := newFixture(t)
	f.wonFrom(21, f.liveSeat("ana"), f.liveSeat("ben"), f.liveSeat("cleo"))
	today := clocktest.Now(t)
	f.at = today
	deal := f.deal(f.env.Rep1, 1_000_000, &today)
	shown := f.chances(deal, today)

	f.showIndividualPerformance(false)
	withheld := f.chances(deal, today)
	if len(shown) != 9 || !maps.Equal(shown, withheld) || shown["deal read"] <= 20 {
		t.Errorf("with the switch off the surfaces read %v, want %v as with it on, above the typed 20%%", withheld, shown)
	}
	if n := f.firstStageEvidence(); n != 21 {
		t.Errorf("the stage chances show %d closed entries, want the 21 of three colleagues", n)
	}
}

func TestAQuietEffectLearnedFromALoneColleaguesDealsFallsBackWhileTheSwitchIsOff(t *testing.T) {
	f := newFixture(t)
	// A year back, so no forecast of this quarter counts the settled deals.
	f.at = clocktest.Now(t).AddDate(-1, 0, 0)
	var rows []forecasting.Contribution
	var settled []ids.DealID
	for range forecasting.QuietSampleFloor {
		q, c := f.deal(f.env.Rep1, 0, nil), f.deal(f.env.Rep1, 0, nil)
		settled = append(settled, q, c)
		rows = append(rows,
			forecasting.Contribution{DealID: q.String(), InOpen: true, Signals: []forecasting.Signal{{Code: assurance.SignalQuiet}}},
			forecasting.Contribution{DealID: c.String(), InOpen: true, Signals: []forecasting.Signal{}})
	}
	f.freeze(f.at, f.at, rows, assurance.RiskSignalsVersion)
	for i, deal := range settled {
		if i%2 == 0 {
			f.close(deal, f.lost)
		} else {
			f.close(deal, f.won)
		}
		if _, err := f.owner.Exec(context.Background(), `UPDATE deal SET owner_id = $2 WHERE id = $1`, deal.UUID, f.env.Rep2); err != nil {
			t.Fatalf("handing the deal on: %v", err)
		}
	}
	today := clocktest.Now(t)
	f.at = today
	deal := f.quietDeal(today)
	learned := func() bool {
		read, err := f.store.GetDeal(f.reader, ids.From[ids.DealKind](deal.UUID), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		chance := read.WinChance
		if chance == nil || chance.EffectLearned == nil {
			t.Fatalf("win chance %+v names no effect on a quiet deal", chance)
		}
		return *chance.EffectLearned
	}
	if !learned() {
		t.Fatal("the quiet effect is not learned from the colleague's settled deals while the switch is on")
	}
	f.showIndividualPerformance(false)
	if learned() {
		t.Error("the quiet effect is learned from one colleague's settled deals while the switch is off")
	}
	requireOneChance(t, f.chances(deal, today), 10)
}
