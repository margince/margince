// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The two interval estimates the verdict rule reads, both one-sided at the one
// confidence level thresholds.go names. Standard textbook forms on purpose: a
// reader arguing with a grade should be able to redo it by hand.

import "math"

// wilsonBounds is the Wilson score interval for passed of runs at confidenceZ.
//
// Wilson rather than the normal approximation because the rule lives at the
// edges: at 3 of 3 the normal interval collapses to the single point 1.0 and
// claims a certainty three runs never bought.
func wilsonBounds(passed, runs int) (lower, upper float64) {
	if runs == 0 {
		return 0, 1
	}
	n := float64(runs)
	p := float64(passed) / n
	z2 := confidenceZ * confidenceZ
	centre := p + z2/(2*n)
	radius := confidenceZ * math.Sqrt(p*(1-p)/n+z2/(4*n*n))
	denom := 1 + z2/n
	return (centre - radius) / denom, (centre + radius) / denom
}

// meanBounds is the t interval on the mean of xs, with its small-n quantile and
// a standard deviation of at least judgeScoreSDFloor, so identical scores keep
// the doubt the judge's coarse steps leave in them.
//
// One value has no degrees of freedom for a t quantile, so its interval is the
// value itself.
func meanBounds(xs []float64) (lower, upper float64) {
	mean, sd := meanAndSD(xs)
	if len(xs) < 2 {
		return mean, mean
	}
	half := tQuantile(len(xs)-1) * max(sd, judgeScoreSDFloor) / math.Sqrt(float64(len(xs)))
	return mean - half, mean + half
}

// meanAndSD answers the mean and the sample (n−1) standard deviation of xs,
// zero for either that the count cannot support.
func meanAndSD(xs []float64) (mean, sd float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	if len(xs) < 2 {
		return mean, 0
	}
	var squares float64
	for _, x := range xs {
		squares += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(squares / float64(len(xs)-1))
}

// tQuantile is tQuantile90 at df degrees of freedom, holding its last entry
// past the table's end.
func tQuantile(df int) float64 {
	if df > len(tQuantile90) {
		df = len(tQuantile90)
	}
	return tQuantile90[df-1]
}
