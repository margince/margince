// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"math"
	"testing"
)

// The bounds against hand-computed values, so a reader re-deriving a grade
// from the page's formulas lands on the numbers the code used.
func TestTheBoundsMatchTheirTextbookValues(t *testing.T) {
	near := func(name string, got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 5e-4 {
			t.Errorf("%s = %.4f, want %.4f", name, got, want)
		}
	}
	lower, upper := wilsonBounds(3, 3)
	near("Wilson lower of 3/3", lower, 0.6462)
	near("Wilson upper of 3/3", upper, 1)
	lower, _ = wilsonBounds(9, 9)
	near("Wilson lower of 9/9", lower, 0.8457)
	lower, _ = wilsonBounds(27, 30)
	near("Wilson lower of 27/30", lower, 0.8078)
	_, upper = wilsonBounds(0, 3)
	near("Wilson upper of 0/3", upper, 0.3538)

	// 60, 65, 90: mean 215/3, variance 775/3, t(0.90, 2) = 1.886.
	mean, sd := 215.0/3, math.Sqrt(775.0/3)
	lower, upper = meanBounds([]float64{60, 65, 90})
	near("t lower of 60,65,90", lower, mean-1.886*sd/math.Sqrt(3))
	near("t upper of 60,65,90", upper, mean+1.886*sd/math.Sqrt(3))

	lower, upper = meanBounds([]float64{42})
	if lower != 42 || upper != 42 {
		t.Errorf("one value's interval = [%v, %v], want the value itself", lower, upper)
	}
	if got := tQuantile(200); got != tQuantile90[len(tQuantile90)-1] {
		t.Errorf("t past the table = %v, want its last, wider entry", got)
	}
}

// Identical scores are not certainty: a judge that scores in steps of about ten
// could as easily have given 61 as 71, so the bound keeps a floor of doubt.
func TestIdenticalScoresStillCarryTheJudgesDoubt(t *testing.T) {
	lower, upper := meanBounds([]float64{1, 1, 1})
	half := tQuantile(2) * judgeScoreSDFloor / math.Sqrt(3)
	if math.Abs(lower-(1-half)) > 1e-9 || math.Abs(upper-(1+half)) > 1e-9 {
		t.Errorf("bounds of 1,1,1 = [%v, %v], want 1 ± %v", lower, upper, half)
	}
	three71 := caseStats{runs: 3, passed: 3, scores: []int{71, 71, 71}, bands: Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}}
	if got := judgeBand([]caseStats{three71}); got == VerdictCertified {
		t.Errorf("three scores of 71 against a bar of 70 reached %s on the judge alone", got)
	}
}
