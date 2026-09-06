// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// The outlook is FROZEN, and the reason is retention.
//
// A review holding only snapshot ids reads as a blank outlook the day those
// snapshots age out, and blank is indistinguishable from a week nobody
// measured. These cases hold the copies: what a review reports must not change
// when the rows it was read from are gone.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stubForecast is a ForecastWeek that answers with whatever it was handed.
//
// A stub is right HERE and nowhere near the arithmetic: this suite is about
// what the review does with an outlook, not about how forecasting computes one.
// The figures below are therefore fixtures, not a second implementation.
type stubForecast struct {
	outlooks  []Outlook
	movements []Movement
	drivers   []Driver
	calls     int
	// teamCalls and teamID record what the TEAM path asked for, which is the
	// one thing a stub can prove about it: the seam is what decides the book,
	// and a team snapshot that quietly took the rep's would look identical
	// here without this.
	teamCalls int
	teamID    ids.UUID
}

func (s *stubForecast) CloseWeek(
	_ context.Context, _ pgx.Tx, _, _ time.Time,
) ([]Outlook, []Movement, []Driver, error) {
	s.calls++
	return s.outlooks, s.movements, s.drivers, nil
}

func (s *stubForecast) CloseTeamWeek(
	_ context.Context, _ pgx.Tx, teamID ids.UUID, _, _ time.Time,
) ([]Outlook, []Movement, []Driver, error) {
	s.teamCalls++
	s.teamID = teamID
	return s.outlooks, s.movements, s.drivers, nil
}

func oneHorizon(openingID, closingID ids.UUID) *stubForecast {
	return &stubForecast{
		outlooks: []Outlook{{
			PeriodKind:     "quarter",
			PeriodStart:    time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:      time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
			Opening:        OutlookSide{Known: true, SnapshotID: openingID, LandingMinor: 300_00},
			Closing:        OutlookSide{Known: true, SnapshotID: closingID, LandingMinor: 420_00},
			ForwardMeasure: "commit_evidence", BaseCurrency: "EUR",
			WonMinor: 180_00, CommitMinor: 120_00,
			BestCaseMinor: 390_00, WeightedMinor: 240_00,
		}},
		movements: []Movement{
			{PeriodKind: "quarter", Bar: BarCreated, DeltaMinor: 90_00},
			{PeriodKind: "quarter", Bar: BarSlipped, DeltaMinor: -30_00},
			{PeriodKind: "quarter", Bar: BarWon, DeltaMinor: 60_00},
		},
		drivers: []Driver{
			{
				PeriodKind: "quarter", Bar: BarCreated, DealID: ids.NewV7(),
				DealLabel: "Nordwind expansion", DeltaMinor: 90_00,
			},
		},
	}
}

// THE POINT OF THE TABLE. Retention removes the snapshots; the review must
// still report the same figures, because a retrospective that empties out is
// not a record.
func TestDeletingTheSnapshotsLeavesTheReviewsOutlookReadable(t *testing.T) {
	e := setupWeekly(t)
	opening, closing := ids.NewV7(), ids.NewV7()
	e.engine = e.engine.WithForecast(oneHorizon(opening, closing))

	written, created, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}
	if !created {
		t.Fatal("the week was not created, so this test says nothing about its outlook")
	}
	if len(written.Outlook) != 1 {
		t.Fatalf("the review froze %d horizons, want 1", len(written.Outlook))
	}

	// The snapshots never existed in this suite — the ids are dangling by
	// construction, which is exactly the state retention leaves behind.
	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatalf("reading the review back: %v", err)
	}
	if len(read.Outlook) != 1 {
		t.Fatalf("the review read back %d horizons, want 1 — a review that empties out "+
			"when its snapshots go is a pointer, not a record", len(read.Outlook))
	}
	got := read.Outlook[0]
	if got.Closing.LandingMinor != 420_00 {
		t.Errorf("the closing landing read back as %d, want 42000", got.Closing.LandingMinor)
	}
	if got.ForwardMeasure != "commit_evidence" {
		t.Errorf("the frozen measure read back as %q, want commit_evidence — a past week "+
			"must keep saying what it was read under", got.ForwardMeasure)
	}
	if got.BestCaseMinor != 390_00 {
		t.Errorf("best case read back as %d, want 39000", got.BestCaseMinor)
	}
}

// An absent Monday snapshot is an ABSENCE, never a zero. A zero opening draws a
// week that started from nothing and made everything.
func TestAMissingMondaySnapshotLeavesTheOpeningSideAbsentNotSubstituted(t *testing.T) {
	e := setupWeekly(t)
	stub := oneHorizon(ids.Nil, ids.NewV7())
	stub.outlooks[0].Opening = OutlookSide{}
	e.engine = e.engine.WithForecast(stub)

	written, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}
	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatalf("reading the review back: %v", err)
	}
	if len(read.Outlook) != 1 {
		t.Fatalf("the review froze %d horizons, want 1", len(read.Outlook))
	}
	if read.Outlook[0].Opening.Known {
		t.Error("the opening side came back KNOWN with no Monday snapshot — the panel " +
			"would draw a zero opening, which reads as a week that started from nothing")
	}
	if read.Outlook[0].Closing.LandingMinor != 420_00 {
		t.Error("the closing side did not survive an absent opening, so an absence on " +
			"one end silently cost the other")
	}
}

// A waterfall read left to right is a story, so two readers of one week must
// get the same one. The bars are stored unordered and come back in the bridge's
// own drawing order.
func TestTheFrozenBarsComeBackInDrawingOrder(t *testing.T) {
	e := setupWeekly(t)
	stub := oneHorizon(ids.NewV7(), ids.NewV7())
	// Written in an order no reader should ever see.
	stub.movements = []Movement{
		{PeriodKind: "quarter", Bar: BarWon, DeltaMinor: 60_00},
		{PeriodKind: "quarter", Bar: BarCreated, DeltaMinor: 90_00},
		{PeriodKind: "quarter", Bar: BarSlipped, DeltaMinor: -30_00},
	}
	e.engine = e.engine.WithForecast(stub)

	written, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}
	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatalf("reading the review back: %v", err)
	}
	if len(read.Outlook) != 1 {
		t.Fatalf("the review froze %d horizons, want 1", len(read.Outlook))
	}

	got := make([]string, 0, len(read.Outlook[0].Movement))
	for _, bar := range read.Outlook[0].Movement {
		got = append(got, bar.Bar)
	}
	want := []string{BarCreated, BarSlipped, BarWon}
	if len(got) != len(want) {
		t.Fatalf("the bridge came back with %d bars (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bar %d came back as %q, want %q — the bars read as a story, and "+
				"two readers of one week must get the same one (got %v)", i, got[i], want[i], got)
		}
	}
}

// An installation with no forecast composed still gets its review. The
// retrospective of what a rep DID does not depend on forecasting anything.
func TestAnAbsentForecastSeamStillWritesTheReview(t *testing.T) {
	e := setupWeekly(t)

	written, created, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("a review with no forecast seam failed instead of being written: %v", err)
	}
	if !created {
		t.Fatal("no review was created")
	}
	if len(written.Outlook) != 0 {
		t.Errorf("a review with no forecast seam carries %d horizons, want none",
			len(written.Outlook))
	}
}

// A second ASSEMBLY writes no second outlook, because the review insert's own
// arbiter returns first. This case holds that path — the cheap one, and the one
// the dispatcher's extra ticks actually take.
func TestASecondAssemblyWritesNoSecondOutlook(t *testing.T) {
	e := setupWeekly(t)
	e.engine = e.engine.WithForecast(oneHorizon(ids.NewV7(), ids.NewV7()))

	if _, _, err := e.engine.AssembleFor(e.repCtx, weekClock); err != nil {
		t.Fatalf("first assembly: %v", err)
	}
	if _, _, err := e.engine.AssembleFor(e.repCtx, weekClock); err != nil {
		t.Fatalf("second assembly: %v", err)
	}

	bars := e.WsCount(t, `SELECT count(*) FROM weekly_review_movement`)
	if bars != 3 {
		t.Errorf("two assemblies left %d bars, want the 3 written once — a doubled "+
			"bridge sums to twice the money anybody moved", bars)
	}
}

// The bar arbiter itself, exercised where it actually bites: a write that
// REACHES the outlook with the rows already present. The assembly path above
// returns before this, so without a direct case the ON CONFLICT is untested and
// a retry inside one transaction would double every bar.
func TestFreezingAnOutlookTwiceLeavesOneSetOfBars(t *testing.T) {
	e := setupWeekly(t)
	stub := oneHorizon(ids.NewV7(), ids.NewV7())
	e.engine = e.engine.WithForecast(stub)

	written, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}

	// The same freeze again, against the review that already has one.
	if err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		return writeOutlook(e.repCtx, tx, written.ID,
			stub.outlooks, stub.movements, stub.drivers)
	}); err != nil {
		t.Fatalf("re-freezing the outlook: %v", err)
	}

	bars := e.WsCount(t, `SELECT count(*) FROM weekly_review_movement`)
	if bars != 3 {
		t.Errorf("freezing twice left %d bars, want 3 — a bridge with two 'created' "+
			"rows sums to twice the pipeline anybody made", bars)
	}
	horizons := e.WsCount(t, `SELECT count(*) FROM weekly_review_outlook`)
	if horizons != 1 {
		t.Errorf("freezing twice left %d horizons, want 1 — two rows for one window "+
			"give the panel two answers and no way to choose", horizons)
	}
}
