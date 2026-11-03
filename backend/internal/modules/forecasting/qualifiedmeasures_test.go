// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// The qualified forecast as its own set of answers: present whenever a policy
// is enforced, its own landing measures, and its own movement causes.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

func TestEnforcedEmptyScopeSendsZeroQualifiedBlock(t *testing.T) {
	t.Parallel()
	period := Period{Start: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Zone: time.UTC}
	enforced, err := Compute(period, period.Start, nil)
	if err != nil {
		t.Fatal(err)
	}
	enforced.ApplyQualifiedPolicy(viability.PolicyVersion)
	if enforced.Qualified == nil || *enforced.Qualified != (QualifiedReadings{}) {
		t.Fatalf("an enforced scope with no deals answered %+v, want a zero block", enforced.Qualified)
	}
	if enforced.QualificationMode() != QualificationEnforcing {
		t.Errorf("mode = %q, want enforcing", enforced.QualificationMode())
	}

	off, err := Compute(period, period.Start, nil)
	if err != nil {
		t.Fatal(err)
	}
	off.ApplyQualifiedPolicy("")
	if off.Qualified != nil || off.QualificationMode() != QualificationOff {
		t.Fatalf("with enforcement off the read answered %+v in mode %q, want no block and off",
			off.Qualified, off.QualificationMode())
	}
}

func TestQualifiedLandingFallsBackWithCaveat(t *testing.T) {
	t.Parallel()
	for measure, baseline := range map[ForwardMeasure]ForwardMeasure{
		MeasureQualifiedCommitEvidence: MeasureCommitEvidence,
		MeasureQualifiedWeighted:       MeasureWeighted,
	} {
		got, err := ProjectLanding(measured(400_00, 250_00, 180_00), measure, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Measure != baseline || got.Caveat != CaveatQualifiedUnavailable {
			t.Errorf("%s with no qualified block landed on %s with caveat %q, want %s and %q",
				measure, got.Measure, got.Caveat, baseline, CaveatQualifiedUnavailable)
		}
	}
	got, err := ProjectLanding(measured(400_00, 250_00, 180_00), MeasureQualifiedCommitEvidence, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountMinor != 650_00 {
		t.Errorf("the fallback landing = %d, want the baseline 65000", got.AmountMinor)
	}
}

func TestQualifiedLandingUsesTheQualifiedReadings(t *testing.T) {
	t.Parallel()
	readings := measured(400_00, 250_00, 180_00)
	readings.Qualified = &QualifiedReadings{EvidenceMinor: 100_00, WeightedMinor: 90_00}
	for measure, want := range map[ForwardMeasure]int64{
		MeasureQualifiedCommitEvidence: 500_00,
		MeasureQualifiedWeighted:       490_00,
	} {
		got, err := ProjectLanding(readings, measure, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.AmountMinor != want || got.Measure != measure || got.Caveat != "" {
			t.Errorf("%s landed %d on %s caveat %q, want %d on itself and no caveat",
				measure, got.AmountMinor, got.Measure, got.Caveat, want)
		}
	}
}

// qualified sets a contribution's qualification and the basis it was frozen on.
func qualified(c Contribution, q viability.Qualification) Contribution {
	c.Qualification = q
	c.Basis = &ViabilityBasis{
		PolicyVersion: viability.PolicyVersion, Disposition: DispositionNone,
		Complete: q != viability.QualificationUnknown,
	}
	return c
}

func policySide(policy string, rows ...Contribution) snapshotSide {
	return snapshotSide{DefinitionVersion: "v1", QualifiedPolicy: policy, Contributions: rows}
}

func TestQualifiedMovementAgainstOldSnapshotIsDefinition(t *testing.T) {
	t.Parallel()
	was := qualified(open("a", 100_00), viability.QualificationEligible)
	now := qualified(open("a", 150_00), viability.QualificationEligible)
	for name, from := range map[string]snapshotSide{
		"frozen with no policy":    policySide("", was),
		"frozen under another one": policySide("viability-0", was),
	} {
		moved := Classify(ReadingQualifiedEvidence, from, policySide(viability.PolicyVersion, now))
		if len(moved.Buckets) != 1 || moved.Buckets[0].Name != BucketDefinition ||
			moved.Buckets[0].AmountMinor != 50_00 {
			t.Errorf("%s: buckets %+v, want the whole 5000 in definition", name, moved.Buckets)
		}
		baseline := Classify(ReadingEvidence, from, policySide(viability.PolicyVersion, now))
		if bucketOf(t, baseline, "a").Bucket != BucketAmount {
			t.Errorf("%s: the baseline reading moved %+v, want the amount change unchanged", name, baseline.Buckets)
		}
	}
}

func TestExcludedAndReinstatedBuckets(t *testing.T) {
	t.Parallel()
	from := policySide(viability.PolicyVersion,
		qualified(open("leaves", 100_00), viability.QualificationEligible),
		qualified(open("returns", 70_00), viability.QualificationExcluded),
		qualified(open("unread", 40_00), viability.QualificationEligible),
		qualified(open("stays", 30_00), viability.QualificationEligible),
	)
	to := policySide(viability.PolicyVersion,
		qualified(open("leaves", 100_00), viability.QualificationUnsupported),
		qualified(open("returns", 70_00), viability.QualificationEligible),
		qualified(open("unread", 40_00), viability.QualificationUnknown),
		qualified(open("stays", 30_00), viability.QualificationEligible),
	)
	moved := Classify(ReadingQualifiedEvidence, from, to)
	reconciles(t, moved)
	for deal, want := range map[string]string{
		"leaves": BucketExcluded, "returns": BucketReinstated, "unread": BucketNotChecked,
	} {
		if got := bucketOf(t, moved, deal); got.Bucket != want {
			t.Errorf("deal %s landed in %q, want %q", deal, got.Bucket, want)
		}
	}
	// The baseline reading does not move with a review.
	if baseline := Classify(ReadingEvidence, from, to); len(baseline.Deals) != 0 {
		t.Errorf("the baseline reading moved %+v on review changes alone", baseline.Deals)
	}
}

func TestADealThatLeftTheQualifiedReadingByDateIsPushedOut(t *testing.T) {
	t.Parallel()
	gone := qualified(open("a", 100_00), viability.QualificationExcluded)
	gone.InOpen, gone.InEvidence, gone.InBestCase = false, false, false
	moved := Classify(ReadingQualifiedEvidence,
		policySide(viability.PolicyVersion, qualified(open("a", 100_00), viability.QualificationEligible)),
		policySide(viability.PolicyVersion, gone))
	if got := bucketOf(t, moved, "a"); got.Bucket != BucketPushedOut {
		t.Errorf("a deal whose date left the period landed in %q, want pushed_out", got.Bucket)
	}
}

func TestARowFrozenWithoutABasisMakesAQualifiedMovementADefinitionChange(t *testing.T) {
	t.Parallel()
	now := qualified(open("a", 100_00), viability.QualificationExcluded)
	unframed := qualified(open("a", 100_00), viability.QualificationEligible)
	unframed.Basis = nil
	otherPolicy := qualified(open("a", 100_00), viability.QualificationEligible)
	otherPolicy.Basis.PolicyVersion = "viability-0"
	for name, was := range map[string]Contribution{
		"a row frozen with no basis":     unframed,
		"a row frozen under another one": otherPolicy,
	} {
		moved := Classify(ReadingQualifiedEvidence,
			policySide(viability.PolicyVersion, was), policySide(viability.PolicyVersion, now))
		if len(moved.Buckets) != 1 || moved.Buckets[0].Name != BucketDefinition {
			t.Errorf("%s: buckets %+v, want the whole difference in definition", name, moved.Buckets)
		}
	}
}

func TestQualifiedOfRebuildsTheBlockFromFrozenRows(t *testing.T) {
	t.Parallel()
	rows := []Contribution{
		qualified(open("kept", 100_00), viability.QualificationEligible),
		qualified(open("out", 40_00), viability.QualificationExcluded),
	}
	got, err := QualifiedOf(rows)
	if err != nil {
		t.Fatal(err)
	}
	if got.EvidenceMinor != 100_00 || got.ExcludedMinor != 40_00 || got.ExcludedCount != 1 {
		t.Fatalf("rebuilt %+v, want 10000 evidence and 4000 excluded", got)
	}
	if empty, err := QualifiedOf(nil); err != nil || empty != (QualifiedReadings{}) {
		t.Fatalf("a snapshot with no rows answered %+v, %v; want a zero block", empty, err)
	}
}
