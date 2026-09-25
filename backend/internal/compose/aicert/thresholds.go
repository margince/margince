// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Every threshold a grade is reached by, so changing what a grade means is an
// edit here; a case's quality bands live in its own corpus file.

// certifiedPassPercent is the pooled pass rate a task must reach to certify: a
// rate rather than every run, so a large pool absorbs a stray miss.
const certifiedPassPercent = 90

// certifiedPassBoundPercent is how low the pooled pass rate's Wilson lower
// bound may sit, so a pool too small to tell 90% from 70% cannot certify.
const certifiedPassBoundPercent = 80

// casePassPercent is the share of its own runs every case must pass for the
// task to certify, so one case failing systematically cannot hide in a pool.
const casePassPercent = 50

// vetoPassPercent is the pass rate a case's Wilson upper bound must reach: a
// case below it is clearly broken, and blocks both upper grades on its own.
const vetoPassPercent = 50

// A task reaches supported_degraded with majorityNumerator/majorityDenominator
// of its pooled runs passing.
const (
	majorityNumerator   = 2
	majorityDenominator = 3
)

// confidenceZ is the standard normal quantile of every one-sided 90% bound the
// rule draws; one level for every bound keeps the rule sayable in one sentence.
const confidenceZ = 1.2816

// tQuantile90 is Student's t one-sided 90% quantile by degrees of freedom
// 1..30, the small-n correction on a mean's bound. Past 30 the last entry
// stands: it is wider than the true quantile, so it can only withhold a grade.
var tQuantile90 = [...]float64{
	3.078, 1.886, 1.638, 1.533, 1.476, 1.440, 1.415, 1.397, 1.383, 1.372,
	1.363, 1.356, 1.350, 1.345, 1.341, 1.337, 1.333, 1.330, 1.328, 1.325,
	1.323, 1.321, 1.319, 1.318, 1.316, 1.315, 1.314, 1.313, 1.311, 1.310,
}

// defaultRepeats is each case's first round of runs unless RUNS= says otherwise.
const defaultRepeats = 3

// adaptiveRound is how many runs a borderline case gains per extension, and
// adaptiveMaxRuns the most any case reaches. Every run pays judgeOpinions judge
// calls, so a case costs at most 9 candidate and 27 judge calls: nine times the
// judge spend of three runs graded once.
const (
	adaptiveRound   = 3
	adaptiveMaxRuns = 9
)

// judgeOpinions is how many times the judge grades every run; the run scores
// at their median. Three is the fewest whose median outvotes one outlier.
const judgeOpinions = 3

// judgeScoreSDFloor is the least standard deviation, in points, a mean's bound
// assumes. The judge scores in steps of about ten, so three identical scores
// say "around here", not "exactly here": half a step is the doubt they keep.
const judgeScoreSDFloor = 5
