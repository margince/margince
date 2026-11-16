// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"
)

// growthSample is one timed read: the time the seat waited, the database's
// share of it, and the round trips it took.
type growthSample struct {
	Elapsed, Server time.Duration
	Trips           int64
}

// growthStep is one read measured at one size of history.
type growthStep struct {
	Years      int
	Cold, Warm []growthSample
}

// growthSeries is one Home read over every measured step.
type growthSeries struct {
	Read  string
	Steps []growthStep
}

// growthLine is a straight line over years of history.
type growthLine struct{ At, PerYear float64 }

func (l growthLine) value(years float64) float64 { return l.At + l.PerYear*years }

// fitGrowthLine is the least-squares line through the points. Two distinct
// years are the least that give it a slope.
func fitGrowthLine(years, values []float64) (growthLine, error) {
	if len(years) != len(values) || len(years) < 2 {
		return growthLine{}, fmt.Errorf("a growth fit needs two or more steps, got %d", len(years))
	}
	n := float64(len(years))
	var sx, sy, sxx, sxy float64
	for i, x := range years {
		sx, sy, sxx, sxy = sx+x, sy+values[i], sxx+x*x, sxy+x*values[i]
	}
	spread := n*sxx - sx*sx
	if spread == 0 {
		return growthLine{}, errors.New("a growth fit needs two different years")
	}
	per := (n*sxy - sx*sy) / spread
	return growthLine{At: (sy - per*sx) / n, PerYear: per}, nil
}

// growthModel is the PoC's projection of one read: T ≈ k·W + r·R + A. W is the
// database's time, R its round trips, A the app's own time, each a line over
// years. k stretches the database's time, as a server out of CPU credit does.
type growthModel struct {
	Work, Trips, Rest growthLine
}

// growthAssumptions are what a projection holds fixed.
type growthAssumptions struct {
	// RoundTrip is r: the delay the bench added to every round trip.
	RoundTrip time.Duration
	// Stretch is k: 1 for a server with its CPU to spare.
	Stretch float64
}

// fitGrowthModel fits the model to the cold reads, the slowest a seat waits.
// A is what is left of each read's median time once W and r·R are taken out.
func fitGrowthModel(steps []growthStep, r time.Duration) (growthModel, error) {
	var years, work, trips, rest []float64
	for _, step := range steps {
		if len(step.Cold) == 0 {
			return growthModel{}, fmt.Errorf("year %d has no cold sample", step.Years)
		}
		elapsed, server, count := growthMedians(step.Cold)
		years = append(years, float64(step.Years))
		work = append(work, server.Seconds())
		trips = append(trips, count)
		rest = append(rest, elapsed.Seconds()-server.Seconds()-count*r.Seconds())
	}
	var m growthModel
	var err error
	if m.Work, err = fitGrowthLine(years, work); err != nil {
		return m, err
	}
	if m.Trips, err = fitGrowthLine(years, trips); err != nil {
		return m, err
	}
	m.Rest, err = fitGrowthLine(years, rest)
	return m, err
}

// predict is the read's time at the given years of history.
func (m growthModel) predict(years float64, a growthAssumptions) time.Duration {
	seconds := a.Stretch*m.Work.value(years) + a.RoundTrip.Seconds()*m.Trips.value(years) + m.Rest.value(years)
	return time.Duration(math.Round(seconds * float64(time.Second)))
}

// growthSearchYears bounds the search for a crossing: further out, the
// projection says nothing about the product.
const growthSearchYears = 100

// crossing is the first whole year whose predicted read reaches line, and
// false when none does within growthSearchYears.
func (m growthModel) crossing(line time.Duration, a growthAssumptions) (int, bool) {
	for years := 1; years <= growthSearchYears; years++ {
		if m.predict(float64(years), a) >= line {
			return years, true
		}
	}
	return 0, false
}

// growthMedians are the medians of the samples' three measures.
func growthMedians(samples []growthSample) (elapsed, server time.Duration, trips float64) {
	e, s := make([]time.Duration, len(samples)), make([]time.Duration, len(samples))
	n := make([]float64, len(samples))
	for i, sample := range samples {
		e[i], s[i], n[i] = sample.Elapsed, sample.Server, float64(sample.Trips)
	}
	slices.Sort(e)
	slices.Sort(s)
	slices.Sort(n)
	mid := len(samples) / 2
	return e[mid], s[mid], n[mid]
}

// growthP90 is the nearest-rank 90th percentile of the samples' times.
func growthP90(samples []growthSample) time.Duration {
	times := make([]time.Duration, len(samples))
	for i, sample := range samples {
		times[i] = sample.Elapsed
	}
	slices.Sort(times)
	return times[int(math.Ceil(0.9*float64(len(times))))-1]
}

// growthLimits are the budgets the growth bench holds.
type growthLimits struct {
	// Line is the read time a seat must never reach. Horizon is how many
	// years past the last measured one the prediction must stay under it.
	Line    time.Duration
	Horizon int
	// WarmServer bounds the database's time for one warm read: the CPU a read
	// costs the server, since a warm read waits on no disk.
	WarmServer time.Duration
	// ConnectionMemory bounds what one server connection still holds after
	// Home's reads, in bytes.
	ConnectionMemory int64
	// CacheHits is the least share of repeat opens the Home cache must serve.
	CacheHits float64
}

// growthOther is what the bench measured besides the read times.
type growthOther struct {
	// ConnectionMemory is the most any connection held, per step, in bytes. A
	// step with no entry measured none.
	ConnectionMemory map[int]int64
	// Cache counts each step's repeat opens, judged step by step so a cache
	// that breaks only at the largest step cannot average into a pass.
	Cache map[int]growthCache
}

// growthCache is one step's repeat opens and how many the cache served.
type growthCache struct {
	Hits, Opens int
}

// growthFailures names every budget the run broke. A run that measured too
// little to judge fails too, so a short census never reads as a pass.
func growthFailures(series []growthSeries, other growthOther, limits growthLimits, a growthAssumptions) []string {
	if len(series) == 0 {
		return []string{"no Home read was measured"}
	}
	var failures []string
	for _, s := range series {
		failures = append(failures, growthReadFailures(s, limits, a)...)
	}
	for _, years := range measuredYears(series) {
		failures = append(failures, growthStepFailures(years, other, limits)...)
	}
	return failures
}

// measuredYears are the steps any read was measured at.
func measuredYears(series []growthSeries) []int {
	seen := map[int]bool{}
	for _, s := range series {
		for _, step := range s.Steps {
			seen[step.Years] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// growthStepFailures judges one step's connection memory and cache. A step
// that measured neither fails, so a missing reading never reads as zero.
func growthStepFailures(years int, other growthOther, limits growthLimits) []string {
	var failures []string
	switch held, measured := other.ConnectionMemory[years]; {
	case !measured:
		failures = append(failures, fmt.Sprintf("year %d: no connection's memory was measured", years))
	case held > limits.ConnectionMemory:
		failures = append(failures, fmt.Sprintf("year %d: a connection holds %d MB after Home's reads, budget %d MB",
			years, held>>20, limits.ConnectionMemory>>20))
	}
	switch cache := other.Cache[years]; {
	case cache.Opens == 0:
		failures = append(failures, fmt.Sprintf("year %d: no repeat open was measured, so the Home cache's hit rate is unknown", years))
	case float64(cache.Hits) < limits.CacheHits*float64(cache.Opens):
		failures = append(failures, fmt.Sprintf("year %d: the Home cache served %d of %d repeat opens, below the %.0f%% floor",
			years, cache.Hits, cache.Opens, limits.CacheHits*100))
	}
	return failures
}

func growthReadFailures(s growthSeries, limits growthLimits, a growthAssumptions) []string {
	var failures []string
	for _, step := range s.Steps {
		if len(step.Cold) == 0 || len(step.Warm) == 0 {
			failures = append(failures, fmt.Sprintf("%s: year %d has %d cold and %d warm samples", s.Read, step.Years, len(step.Cold), len(step.Warm)))
			continue
		}
		if p90 := growthP90(step.Cold); p90 >= limits.Line {
			failures = append(failures, fmt.Sprintf("%s: year %d: cold p90 %s is at or over %s", s.Read, step.Years, p90.Round(time.Millisecond), limits.Line))
		}
		if _, server, _ := growthMedians(step.Warm); server > limits.WarmServer {
			failures = append(failures, fmt.Sprintf("%s: year %d: a warm read costs the database %s, budget %s",
				s.Read, step.Years, server.Round(time.Millisecond), limits.WarmServer))
		}
	}
	if len(failures) > 0 {
		return failures
	}
	model, err := fitGrowthModel(s.Steps, a.RoundTrip)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", s.Read, err)}
	}
	last := s.Steps[len(s.Steps)-1].Years
	if years, crosses := model.crossing(limits.Line, a); crosses && years <= last+limits.Horizon {
		return []string{fmt.Sprintf("%s: predicted to reach %s at %d years of mail, within %d years of the %d measured",
			s.Read, limits.Line, years, limits.Horizon, last)}
	}
	return nil
}
