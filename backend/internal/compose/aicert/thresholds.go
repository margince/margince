// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Every threshold a grade is reached by, so changing what a grade means is an
// edit here; a case's quality bands live in its own corpus file.

// certifiedPassPercent is the pooled pass rate a set must reach to certify: a
// rate rather than every run, so a large pool absorbs a stray miss.
const certifiedPassPercent = 90

// A case must pass majorityNumerator/majorityDenominator of its own runs to
// certify, and a set that pooled fraction to reach supported_degraded.
const (
	majorityNumerator   = 2
	majorityDenominator = 3
)

// defaultRepeats is how many times each case runs unless RUNS= says otherwise;
// odd, per Verdict's run-count requirement.
const defaultRepeats = 3

// rejudgeOpinions is how many further opinions a score below certified_min is
// weighed against: three in all is the fewest whose median outvotes one outlier.
const rejudgeOpinions = 2
