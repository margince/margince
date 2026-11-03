// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

func TestTheQualifiedReadingsLeaveOutAReviewedDealAndCountTheUnknownApart(t *testing.T) {
	t.Parallel()
	period := testPeriod(t)
	asOf := *day(t, time.May, 14)
	deal := func(id string, amount int64, q viability.Qualification) Deal {
		d := healthyDeal(t)
		d.ID, d.AmountMinor, d.BaseMinor, d.Qualification = id, new(amount), new(amount), q
		return d
	}
	got, err := Compute(period, asOf, []Deal{
		deal("supported", 100_000, viability.QualificationEligible),
		deal("silent", 400_000, viability.QualificationExcluded),
		deal("unread", 20_000, viability.QualificationUnknown),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The baseline is untouched: qualification never rewrites what it sits beside.
	if got.EvidenceMinor != 520_000 || got.WeightedMinor != 260_000 {
		t.Fatalf("baseline evidence %d weighted %d, want 520000 and 260000", got.EvidenceMinor, got.WeightedMinor)
	}
	want := QualifiedReadings{
		OpenMinor: 100_000, EvidenceMinor: 100_000, BestCaseMinor: 100_000, WeightedMinor: 50_000,
		ExcludedMinor: 400_000, UnknownMinor: 20_000, ExcludedCount: 1, UnknownCount: 1,
	}
	if got.Qualified == nil || *got.Qualified != want {
		t.Fatalf("qualified readings %+v, want %+v", got.Qualified, want)
	}
}

func TestNoQualifiedReadingExistsWhileEnforcementIsOff(t *testing.T) {
	t.Parallel()
	got, err := Compute(testPeriod(t), *day(t, time.May, 14), []Deal{healthyDeal(t)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Qualified != nil {
		t.Fatalf("an unqualified read produced qualified readings %+v; they would pass the baseline off as qualified",
			got.Qualified)
	}
}

func TestALeftOutDealWithNoVisibleAmountMarksTheSubtotalsPartial(t *testing.T) {
	t.Parallel()
	masked := healthyDeal(t)
	masked.AmountMinor, masked.BaseMinor, masked.Qualification = nil, nil, viability.QualificationExcluded
	got, err := Compute(testPeriod(t), *day(t, time.May, 14), []Deal{masked})
	if err != nil {
		t.Fatal(err)
	}
	if got.Qualified == nil || !got.Qualified.AmountsPartial || got.Qualified.ExcludedCount != 1 {
		t.Fatalf("qualified readings %+v, want one excluded deal and the subtotal marked partial — "+
			"otherwise the screen says the deal carries no money", got.Qualified)
	}
}
