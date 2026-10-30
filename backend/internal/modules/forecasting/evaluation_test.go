// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// evaluationCorpus is two ended quarters and one open one, cut around a day
// derived from the test clock. The first quarter settles before the training
// window ends; the second ends inside the validation window.
type evaluationCorpus struct {
	day     func(offset int) time.Time
	plan    EvaluationPlan
	dataset EvaluationDataset
	q1, q2  [2]time.Time
	q3Start time.Time
}

func newEvaluationCorpus(t *testing.T) evaluationCorpus {
	t.Helper()
	now := clocktest.Now(t).UTC()
	anchor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	day := func(offset int) time.Time { return anchor.AddDate(0, 0, offset) }
	c := evaluationCorpus{
		day: day, q1: [2]time.Time{day(0), day(89)}, q2: [2]time.Time{day(90), day(179)}, q3Start: day(180),
		plan: EvaluationPlan{
			TrainEnd: day(100), ValidationEnd: day(170), HorizonEdges: []int{30, 60}, SettleDays: 5,
			CohortFloor: 1, ScoreCandidate: true,
		},
	}
	c.dataset = EvaluationDataset{
		Version: EvaluationDatasetVersion, DefinitionVersion: DefinitionVersion,
		OutcomesAsOf: day(200).Add(9 * time.Hour), OutcomesDay: day(200), TrainEnd: day(100), CohortFloor: 1,
		Vintages: []EvaluationVintage{
			c.vintage("a1", c.q1, 10, 100_00, 400_00, 1000_00, 300_00, 900_00),
			c.vintage("a2", c.q1, 70, 300_00, 400_00, 600_00, 200_00, 900_00),
			c.vintage("b1", c.q2, 95, 0, 250_00, 800_00, 100_00, 500_00),
			c.vintage("b2", c.q2, 150, 200_00, 200_00, 500_00, 250_00, 500_00),
			c.vintage("b3", c.q2, 175, 400_00, 50_00, 100_00, 60_00, 500_00),
		},
	}
	return c
}

func (c evaluationCorpus) vintage(id string, period [2]time.Time, takenOn int, won, weighted, open, evidence, outcome int64) EvaluationVintage {
	won10, lost := true, false
	ten, sixty := 10, 60
	deals := []OpenDeal{
		{StageProbability: &ten, WonInHorizon: &lost},
		{StageProbability: &sixty, InEvidence: true, WonInHorizon: &won10},
	}
	return EvaluationVintage{
		ID: id, Context: "workspace::", PeriodStart: period[0], PeriodEnd: period[1],
		LocalDay: c.day(takenOn), TakenAt: c.day(takenOn).Add(6 * time.Hour), Trigger: TriggerDaily,
		Definition: DefinitionVersion, Currency: "EUR", Eligible: 3, Priced: 3, Rows: 3,
		WonMinor: won, Weighted: weighted, OpenMinor: open, Evidence: evidence,
		Tallies: TallyOpenDeals(deals),
		Outcome: &VintageOutcome{WonMinor: outcome}, TrainOutcome: &VintageOutcome{WonMinor: outcome},
	}
}

func scoreCorpus(t *testing.T, dataset EvaluationDataset, plan EvaluationPlan) EvaluationReport {
	t.Helper()
	report, err := Evaluate(dataset, plan)
	if err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	return report
}

func reportJSON(t *testing.T, report EvaluationReport) []byte {
	t.Helper()
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func findRow(t *testing.T, report EvaluationReport, method, window, horizon string) EvaluationRow {
	t.Helper()
	for _, row := range report.Rows {
		if row.Method == method && row.Window == window && row.Horizon == horizon {
			return row
		}
	}
	t.Fatalf("no %s row for %s, %s in %+v", method, window, horizon, report.Rows)
	return EvaluationRow{}
}

func TestAnEvaluationOfTheSameDatasetPrintsTheSameReport(t *testing.T) {
	c := newEvaluationCorpus(t)
	first := reportJSON(t, scoreCorpus(t, c.dataset, c.plan))

	body, err := json.Marshal(c.dataset)
	if err != nil {
		t.Fatal(err)
	}
	var reread EvaluationDataset
	if err := json.Unmarshal(body, &reread); err != nil {
		t.Fatal(err)
	}
	slices.Reverse(reread.Vintages)
	if second := reportJSON(t, scoreCorpus(t, reread, c.plan)); !bytes.Equal(first, second) {
		t.Fatalf("a reread, reordered dataset scored differently:\n%s\n%s", first, second)
	}
}

// A vintage in the training window whose period had not settled when the
// window closed cannot shape the candidate, whatever its period later won.
func TestAnOutcomeKnownAfterTheTrainingWindowNeverShapesTheCandidate(t *testing.T) {
	c := newEvaluationCorpus(t)
	before := scoreCorpus(t, c.dataset, c.plan)

	later := c.dataset
	later.Vintages = slices.Clone(c.dataset.Vintages)
	later.Vintages[2].TrainOutcome = &VintageOutcome{WonMinor: 9_000_00}
	later.Vintages[0].Outcome = &VintageOutcome{WonMinor: 9_000_00}
	after := scoreCorpus(t, later, c.plan)
	if !slices.Equal(fits(before), fits(after)) {
		t.Fatalf("an outcome settled after the training window, or a correction after it, changed the fit: %v then %v",
			fits(before), fits(after))
	}

	settled := c.dataset
	settled.Vintages = slices.Clone(c.dataset.Vintages)
	settled.Vintages[0].TrainOutcome = &VintageOutcome{WonMinor: 9_000_00}
	if moved := scoreCorpus(t, settled, c.plan); slices.Equal(fits(before), fits(moved)) {
		t.Fatal("an outcome settled inside the training window did not reach the fit, so the test above proves nothing")
	}
}

func fits(report EvaluationReport) []string {
	var out []string
	for _, fit := range report.Candidate {
		out = append(out, fit.Horizon+"="+fit.Ratio+fit.NotFit)
	}
	return out
}

func TestTheFrozenFiguresAreScoredWithoutTheOutcomeChangingThem(t *testing.T) {
	c := newEvaluationCorpus(t)
	row := findRow(t, scoreCorpus(t, c.dataset, c.plan), MethodStageWeighting, WindowValidation, "0-29")
	if row.Label != LabelObserved || row.Vintages != 1 || row.Money.PredictedMinor != 400_00 || row.Money.ActualMinor != 500_00 {
		t.Fatalf("b2 is won 200 plus weighted 200 against 500 won, observed; got %+v %+v", row, row.Money)
	}
	if row.Money.SignedErrorMinor != -100_00 || row.Money.AbsErrorPerActual != "0.2000" {
		t.Fatalf("a landing 100 short of 500 won: %+v", row.Money)
	}
	commit := findRow(t, scoreCorpus(t, c.dataset, c.plan), MethodCommitEvidence, WindowValidation, "0-29")
	if commit.Money.PredictedMinor != 450_00 || commit.Calibration.Bins[1].Bin != "committed" {
		t.Fatalf("b2 commits won 200 plus evidence 250: %+v %+v", commit.Money, commit.Calibration)
	}
}

func TestACandidateIsABacktestAndAClosedPeriodIsNoPrediction(t *testing.T) {
	c := newEvaluationCorpus(t)
	closing := c.vintage("q1-close", c.q1, 89, 900_00, 0, 0, 0, 900_00)
	closing.Trigger = TriggerPeriodClose
	late := c.vintage("q1-late", c.q1, 92, 900_00, 0, 0, 0, 900_00)
	c.dataset.Vintages = append(c.dataset.Vintages, closing, late)
	report := scoreCorpus(t, c.dataset, c.plan)

	if got := exclusionsOf(report)[ExcludedAfterOutcome]; got != 2 {
		t.Fatalf("a period-close and an after-the-end snapshot are not predictions, %d excluded", got)
	}
	for _, row := range report.Rows {
		if want := LabelObserved; row.Method == MethodOpenClosure {
			if row.Label != LabelBacktest {
				t.Fatalf("the candidate's row is labelled %q", row.Label)
			}
		} else if row.Label != want {
			t.Fatalf("a frozen reading's row is labelled %q", row.Label)
		}
	}
	if train := findRow(t, report, MethodOpenClosure, WindowTrain, "60+"); train.NotScored == "" || train.Money != nil {
		t.Fatalf("the candidate was scored on the window it was fitted on: %+v", train)
	}
}

func exclusionsOf(report EvaluationReport) map[string]int {
	out := map[string]int{}
	for _, e := range report.Census.Excluded {
		out[e.Reason] = e.Vintages
	}
	return out
}

func TestEveryExcludedVintageIsCountedUnderItsReason(t *testing.T) {
	c := newEvaluationCorpus(t)
	pending := c.vintage("q3", [2]time.Time{c.q3Start, c.day(269)}, 190, 0, 10_00, 50_00, 0, 0)
	pending.Outcome = nil
	oldRules := c.vintage("old", c.q1, 20, 0, 0, 0, 0, 0)
	oldRules.Definition = "forecast_v1"
	erased := c.vintage("erased", c.q1, 30, 0, 0, 0, 0, 0)
	erased.Rows = 0
	unconverted := c.vintage("fx", c.q1, 40, 0, 0, 0, 0, 0)
	unconverted.Outcome.FxMissing = 1
	missing := c.vintage("missing", c.q1, 50, 0, 0, 0, 0, 0)
	missing.Outcome = nil
	c.dataset.Vintages = append(c.dataset.Vintages, pending, oldRules, erased, unconverted, missing)
	c.dataset.LeftOut = []ExclusionCount{{Reason: ExcludedOtherScope, Vintages: 4}}
	report := scoreCorpus(t, c.dataset, c.plan)

	want := map[string]int{
		ExcludedOutcomePending: 1, ExcludedDefinitionChanged: 1, ExcludedEvidenceMissing: 1,
		ExcludedOutcomeIncomplete: 1, ExcludedOutcomeMissing: 1, ExcludedOtherScope: 4,
	}
	got := exclusionsOf(report)
	for reason, n := range want {
		if got[reason] != n {
			t.Errorf("%s: %d excluded, want %d", reason, got[reason], n)
		}
	}
	if report.Census.Vintages != 14 || report.Census.Included != 5 {
		t.Fatalf("census %+v, want 14 vintages and the corpus's 5 included", report.Census)
	}
}

func TestAPeriodThatWonNothingIsScoredWithoutADivision(t *testing.T) {
	c := newEvaluationCorpus(t)
	for i := range c.dataset.Vintages {
		c.dataset.Vintages[i].Outcome = &VintageOutcome{}
	}
	row := findRow(t, scoreCorpus(t, c.dataset, c.plan), MethodStageWeighting, WindowValidation, "0-29")
	if row.Money.AbsErrorPerActual != "" || row.Money.ZeroOutcomeVintages != 1 || row.Money.ZeroOutcomeAbsErrorMinor != 400_00 {
		t.Fatalf("b2 forecast 400 for a period that won nothing: %+v", row.Money)
	}
}

func TestStageWeightingIsCalibratedPerDeal(t *testing.T) {
	c := newEvaluationCorpus(t)
	cal := findRow(t, scoreCorpus(t, c.dataset, c.plan), MethodStageWeighting, WindowValidation, "0-29").Calibration
	// One 10% deal lost and one 60% deal won: ((10-0)² + (60-100)²) / (100² × 2).
	if cal.Deals != 2 || cal.BrierScore != "0.0850" || len(cal.Bins) != 2 || cal.Bins[0].MeanPredicted != "0.1000" || cal.Bins[1].WonShare != "1.0000" {
		t.Fatalf("calibration %+v", cal)
	}
}

func TestAVintageIsAPredictionUntilItsPeriodEnds(t *testing.T) {
	end := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		trigger string
		day     time.Time
		want    PredictionKind
	}{
		{TriggerDaily, end, PredictionObserved},
		{TriggerDaily, end.AddDate(0, 0, 1), PredictionAfterOutcome},
		{TriggerPeriodClose, end, PredictionAfterOutcome},
	}
	for _, tc := range cases {
		if got := PredictionKindOf(tc.trigger, tc.day, end); got != tc.want {
			t.Errorf("%s on %s: %s, want %s", tc.trigger, tc.day.Format(time.DateOnly), got, tc.want)
		}
	}
	if got := HorizonDays(end.AddDate(0, 0, -30), end); got != 30 {
		t.Fatalf("30 days before the end looks %d days ahead", got)
	}
}

func TestWithoutAPreregistrationOnlyTheConfiguredReadingsAreScored(t *testing.T) {
	c := newEvaluationCorpus(t)
	c.plan.ScoreCandidate = false
	report := scoreCorpus(t, c.dataset, c.plan)
	if report.CandidateNotScored == "" || len(report.Candidate) != 0 {
		t.Fatalf("the candidate was fitted with no pre-registration: %+v", report.Candidate)
	}
	for _, row := range report.Rows {
		if row.Method == MethodOpenClosure {
			t.Fatalf("the candidate was scored with no pre-registration: %+v", row)
		}
	}
}

// A bin of fewer deals than the floor says how many it holds and nothing
// about them; a vintage below the floor is not scored at all.
func TestAFigureBelowTheCohortFloorIsWithheld(t *testing.T) {
	c := newEvaluationCorpus(t)
	c.plan.CohortFloor = 2
	cal := findRow(t, scoreCorpus(t, c.dataset, c.plan), MethodStageWeighting, WindowValidation, "0-29").Calibration
	for _, bin := range cal.Bins {
		if bin.Deals != 1 || bin.WonShare != "" || bin.MeanPredicted != "" {
			t.Fatalf("a one-deal bin under a floor of two reads %+v", bin)
		}
	}
	if cal.BrierScore != "0.0850" {
		t.Fatalf("two binned deals meet a floor of two, Brier %q", cal.BrierScore)
	}
	c.plan.CohortFloor = 4
	if got := exclusionsOf(scoreCorpus(t, c.dataset, c.plan))[ExcludedBelowFloor]; got != 5 {
		t.Fatalf("%d vintages of three deals excluded under a floor of four, want 5", got)
	}
}

func TestAPlanMustTrainToTheDayTheLabelsWereReadAt(t *testing.T) {
	c := newEvaluationCorpus(t)
	c.plan.TrainEnd = c.day(101)
	if _, err := Evaluate(c.dataset, c.plan); err == nil {
		t.Fatal("a plan training to another day than the dataset's labels was scored")
	}
}
