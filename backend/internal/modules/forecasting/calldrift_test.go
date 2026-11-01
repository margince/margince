// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// called is a baseline side with everything a call's snapshot records.
func called(rows ...Contribution) snapshotSide {
	return snapshotSide{
		DefinitionVersion: DefinitionVersion, Contributions: rows, BaseCurrency: "EUR",
		PopulationFingerprint: "team-a", InputFingerprint: InputFingerprint(rows),
		QualifiedPolicy: viability.PolicyVersion,
	}
}

func today(rows ...Contribution) snapshotSide {
	return todayOf(rows, "team-a", "EUR", viability.PolicyVersion)
}

func todayOf(rows []Contribution, population, currency, policy string) snapshotSide {
	return TodaySide(Baseline{
		Readings: Readings{Contributions: rows}, PopulationFingerprint: population,
		BaseCurrency: currency, QualifiedPolicy: policy,
	}, time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC))
}

// A deal whose amount, stage and close date all moved is one row. Its causes
// are the two that move money, and opening plus the buckets is closing.
func TestADealThatMovedThreeWaysIsCountedOnce(t *testing.T) {
	t.Parallel()
	was := open("d1", 10_000)
	was.EffectiveClose = new(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))
	now := open("d1", 14_000)
	now.StageProbability = 70
	now.WeightedMinor = 9_800
	now.EffectiveClose = new(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	steady := open("d2", 5_000)

	for _, reading := range everyReading {
		m := Classify(reading, side(DefinitionVersion, was, steady), side(DefinitionVersion, now, steady))
		reconciles(t, m)
		row := bucketOf(t, m, "d1")
		if !slices.Equal(row.Causes, []string{BucketAmount, BucketStageWeight}) {
			t.Errorf("%s: causes = %v, want the amount and the stage weight", reading, row.Causes)
		}
	}

	drift := CompareWithBaseline(called(was, steady), today(now, steady))
	if len(drift.Deals) != 1 || drift.Deals[0].DealID != "d1" {
		t.Fatalf("drift deals = %+v, want d1 once", drift.Deals)
	}
	if drift.Net[ReadingOpen] != 4_000 || drift.Net[ReadingWeighted] != 9_800-5_000 {
		t.Errorf("net = %v, want each reading's own change", drift.Net)
	}
}

// Age alone is not a change: a call nothing moved under stays current, even
// read years later.
func TestACallNothingMovedUnderIsCurrentHoweverOld(t *testing.T) {
	t.Parallel()
	rows := []Contribution{open("d1", 10_000), open("d2", 3_000)}
	later := todayOf(rows, "team-a", "EUR", viability.PolicyVersion)
	later.TakenAt = time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
	if drift := CompareWithBaseline(called(rows...), later); drift.Material() || drift.Limitation != "" {
		t.Fatalf("an unchanged call reads as moved: %+v", drift)
	}
}

// An edit the forecast does not count is not material: a close date that
// moved inside the period, or a new owner.
func TestAnEditTheForecastDoesNotCountIsNotMaterial(t *testing.T) {
	t.Parallel()
	was := open("d1", 10_000)
	was.EffectiveClose = new(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))
	now := was
	now.EffectiveClose = new(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	now.Owner = "someone-else"
	outside := Contribution{DealID: "d9", Category: CategoryCommit}

	if drift := CompareWithBaseline(called(was), today(now, outside)); drift.Material() {
		t.Fatalf("a date inside the period, a new owner and a deal outside it read as material: %+v", drift.Deals)
	}
}

// Leaving the period, and leaving or rejoining the qualified forecast, are
// material even where the money in one reading stays the same.
func TestLeavingThePeriodOrTheQualifiedForecastIsMaterial(t *testing.T) {
	t.Parallel()
	was := open("d1", 10_000)
	was.Qualification = viability.QualificationEligible
	excluded := was
	excluded.Qualification = viability.QualificationExcluded
	pushed := Contribution{DealID: "d2", Category: CategoryCommit, Qualification: viability.QualificationEligible}
	inPeriod := open("d2", 2_000)
	inPeriod.Qualification = viability.QualificationEligible

	drift := CompareWithBaseline(called(was, inPeriod), today(excluded, pushed))
	if len(drift.Deals) != 2 || drift.QualificationChanges != 1 {
		t.Fatalf("drift = %+v, want both deals and one qualification change", drift)
	}
	if !slices.Contains(drift.Deals[0].Causes, CauseQualification) {
		t.Errorf("the excluded deal's causes are %v, want the qualification", drift.Deals[0].Causes)
	}
	if drift.Deals[1].Bucket != BucketPushedOut {
		t.Errorf("the deal that left the period is in %q, want pushed out", drift.Deals[1].Bucket)
	}
}

// A changed basis is a limitation and never a delta.
func TestAChangedBasisIsALimitationNotADelta(t *testing.T) {
	t.Parallel()
	rows := []Contribution{open("d1", 10_000)}
	moved := []Contribution{open("d1", 99_000)}

	for _, tc := range []struct {
		name  string
		today snapshotSide
		want  Limitation
	}{
		{"another team", todayOf(moved, "team-b", "EUR", viability.PolicyVersion), LimitationPopulation},
		{"another base currency", todayOf(moved, "team-a", "USD", viability.PolicyVersion), LimitationCurrency},
		{"qualification switched off", todayOf(moved, "team-a", "EUR", ""), LimitationQualifiedPolicy},
		{"another definition", func() snapshotSide { s := today(moved...); s.DefinitionVersion = "forecast_v1"; return s }(), LimitationDefinition},
	} {
		drift := CompareWithBaseline(called(rows...), tc.today)
		if drift.Limitation != tc.want || len(drift.Deals) != 0 || len(drift.Net) != 0 {
			t.Errorf("%s: drift = %+v, want only the limitation %q", tc.name, drift, tc.want)
		}
	}
}

// The fingerprint is over what the deals contribute, in no particular order.
func TestTheInputFingerprintIgnoresOrderAndSeesQualification(t *testing.T) {
	t.Parallel()
	a, b := open("d1", 10_000), open("d2", 3_000)
	if InputFingerprint([]Contribution{a, b}) != InputFingerprint([]Contribution{b, a}) {
		t.Fatal("the same contributions in another order have another fingerprint")
	}
	excluded := a
	excluded.Qualification = viability.QualificationExcluded
	if InputFingerprint([]Contribution{a, b}) == InputFingerprint([]Contribution{excluded, b}) {
		t.Fatal("a deal leaving the qualified forecast left the fingerprint unchanged")
	}
	// The range draws one chance per stage, so a deal changing stage at the same
	// chance changes what the forecast says.
	moved := a
	moved.StageID = "stage-elsewhere"
	if InputFingerprint([]Contribution{a, b}) == InputFingerprint([]Contribution{moved, b}) {
		t.Fatal("a deal moving to another stage left the fingerprint unchanged")
	}
}

// The policy is the setting's, not the deals': a period whose first deal
// arrives, or whose last one leaves, compares on the same policy.
func TestAnEmptyPeriodGainingADealIsAChangeNotALimitation(t *testing.T) {
	t.Parallel()
	arrived := open("d1", 10_000)
	arrived.Qualification = viability.QualificationEligible

	gained := CompareWithBaseline(called(), today(arrived))
	if gained.Limitation != "" || !gained.Material() {
		t.Fatalf("an empty period gaining a deal = %+v, want a material change", gained)
	}
	emptied := CompareWithBaseline(called(arrived), today())
	if emptied.Limitation != "" || !emptied.Material() {
		t.Fatalf("a period losing its last deal = %+v, want a material change", emptied)
	}
}

// Every Reading declared in movement.go is compared for a call, so a reading
// added there cannot move a forecast without the drift seeing it.
func TestEveryReadingIsComparedForACall(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "movement.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := 0
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || value.Type == nil || fmt.Sprint(value.Type) != "Reading" {
				continue
			}
			for _, lit := range value.Values {
				basic, ok := lit.(*ast.BasicLit)
				if !ok {
					continue
				}
				declared++
				if !slices.Contains(everyReading, Reading(strings.Trim(basic.Value, `"`))) {
					t.Errorf("reading %s is declared and not compared for a call", basic.Value)
				}
			}
		}
	}
	if declared < 4 {
		t.Fatalf("found %d Reading constants in movement.go, want at least the four", declared)
	}
}
