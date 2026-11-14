// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"maps"
	"math"
	"strings"
	"testing"
	"time"
)

var growthTestAssume = growthAssumptions{RoundTrip: 3 * time.Millisecond, Stretch: 1}

var growthTestLimits = growthLimits{
	Line: 10 * time.Second, Horizon: 4, WarmServer: 2 * time.Second, ConnectionMemory: 300 << 20, CacheHits: 0.75,
}

// growthTestSeries is a read whose cold time is at + per·years seconds, all
// of it database time, with 100 round trips each.
func growthTestSeries(at, per float64, years ...int) growthSeries {
	s := growthSeries{Read: "rep_worklist"}
	for _, y := range years {
		cold := time.Duration((at + per*float64(y)) * float64(time.Second))
		sample := growthSample{Elapsed: cold + 300*time.Millisecond, Server: cold, Trips: 100}
		warm := growthSample{Elapsed: 100 * time.Millisecond, Server: 50 * time.Millisecond, Trips: 100}
		s.Steps = append(s.Steps, growthStep{Years: y, Cold: []growthSample{sample}, Warm: []growthSample{warm}})
	}
	return s
}

// growthTestOther passes every step the test series measure, years 1 to 4.
var growthTestOther = growthOther{
	ConnectionMemory: map[int]int64{1: 10 << 20, 2: 10 << 20, 3: 10 << 20, 4: 10 << 20},
	Cache:            map[int]growthCache{1: {4, 5}, 2: {4, 5}, 3: {4, 5}, 4: {4, 5}},
}

// laterStep is the step the budget cases break, between full ones.
const laterStep = 2

// withLaterStep is growthTestOther with laterStep's readings replaced.
func withLaterStep(memory *int64, cache *growthCache) growthOther {
	years := laterStep
	other := growthOther{ConnectionMemory: maps.Clone(growthTestOther.ConnectionMemory), Cache: maps.Clone(growthTestOther.Cache)}
	if memory == nil {
		delete(other.ConnectionMemory, years)
	} else {
		other.ConnectionMemory[years] = *memory
	}
	if cache != nil {
		other.Cache[years] = *cache
	}
	return other
}

func TestAGrowthFitRecoversTheLineThroughItsPoints(t *testing.T) {
	line, err := fitGrowthLine([]float64{1, 2, 3}, []float64{2.5, 3, 3.5})
	if err != nil || math.Abs(line.At-2) > 1e-9 || math.Abs(line.PerYear-0.5) > 1e-9 {
		t.Fatalf("fit = %+v, %v; want 2 + 0.5/yr", line, err)
	}
	for _, years := range [][]float64{{1}, {2, 2}} {
		if _, err := fitGrowthLine(years, make([]float64, len(years))); err == nil {
			t.Errorf("a fit over years %v has no slope and must refuse", years)
		}
	}
}

func TestTheModelSplitsAReadIntoWorkRoundTripsAndTheRest(t *testing.T) {
	model, err := fitGrowthModel(growthTestSeries(1, 1, 1, 2).Steps, growthTestAssume.RoundTrip)
	if err != nil {
		t.Fatal(err)
	}
	// Each read is its work, 100 trips of 3 ms, and no other time.
	if got := model.predict(3, growthTestAssume); got != 4300*time.Millisecond {
		t.Fatalf("predicted %s at 3 years, want 4.3s", got)
	}
	spent := growthAssumptions{RoundTrip: growthTestAssume.RoundTrip, Stretch: 2}
	if got := model.predict(3, spent); got != 8300*time.Millisecond {
		t.Fatalf("predicted %s at 3 years with the CPU stretched twice, want 8.3s", got)
	}
}

func TestAReadThatStaysFlatPasses(t *testing.T) {
	if failures := growthFailures([]growthSeries{growthTestSeries(1, 0, 1, 2)}, growthTestOther, growthTestLimits, growthTestAssume); len(failures) != 0 {
		t.Fatalf("a flat read failed: %v", failures)
	}
}

func TestAReadPredictedToReachTheLineWithinTheHorizonFails(t *testing.T) {
	// 1.6 s at one year and 2 s more each year reach 10 s at year 6: the last
	// year a horizon of four allows past year 2.
	failures := growthFailures([]growthSeries{growthTestSeries(-0.7, 2, 1, 2)}, growthTestOther, growthTestLimits, growthTestAssume)
	if len(failures) != 1 || !strings.Contains(failures[0], "at 6 years") {
		t.Fatalf("want one failure naming year 6, got %v", failures)
	}
	far := growthTestLimits
	far.Horizon = 2
	if failures := growthFailures([]growthSeries{growthTestSeries(-0.7, 2, 1, 2)}, growthTestOther, far, growthTestAssume); len(failures) != 0 {
		t.Fatalf("a crossing past the horizon must pass, got %v", failures)
	}
}

func TestAReadAlreadyOverTheLineFails(t *testing.T) {
	failures := growthFailures([]growthSeries{growthTestSeries(10, 0, 1, 2)}, growthTestOther, growthTestLimits, growthTestAssume)
	if len(failures) != 2 || !strings.Contains(failures[0], "cold p90") {
		t.Fatalf("each step over the line must fail on its own, got %v", failures)
	}
}

func TestOneStepIsTooFewToPredictAndFails(t *testing.T) {
	failures := growthFailures([]growthSeries{growthTestSeries(1, 0, 1)}, growthTestOther, growthTestLimits, growthTestAssume)
	if len(failures) != 1 || !strings.Contains(failures[0], "two or more steps") {
		t.Fatalf("one step must fail as too few to fit, got %v", failures)
	}
}

func TestAStepWithNoSamplesFailsRatherThanPasses(t *testing.T) {
	s := growthTestSeries(1, 0, 1, 2)
	s.Steps[1].Warm = nil
	if failures := growthFailures([]growthSeries{s}, growthTestOther, growthTestLimits, growthTestAssume); len(failures) != 1 {
		t.Fatalf("a step with no warm sample must fail, got %v", failures)
	}
	if failures := growthFailures(nil, growthTestOther, growthTestLimits, growthTestAssume); len(failures) != 1 {
		t.Fatalf("a run that measured nothing must fail, got %v", failures)
	}
}

func TestTheServerMemoryAndCacheBudgetsEachFail(t *testing.T) {
	slow := growthTestSeries(1, 0, 1, 2)
	slow.Steps[0].Warm[0].Server = 3 * time.Second
	ok := int64(10 << 20)
	heavy := withLaterStep(new(int64(400<<20)), nil)
	// Full steps around it cannot carry a step that served nothing.
	cold := withLaterStep(&ok, &growthCache{Hits: 0, Opens: 5})
	unmeasured := withLaterStep(&ok, &growthCache{})
	noMemory := withLaterStep(nil, nil)
	for name, tc := range map[string]struct {
		series growthSeries
		other  growthOther
		want   string
	}{
		"warm server time":  {slow, growthTestOther, "costs the database"},
		"connection memory": {growthTestSeries(1, 0, 1, 2), heavy, "400 MB"},
		"cache hit rate":    {growthTestSeries(1, 0, 1, 2), cold, "0 of 5"},
		"no repeat open":    {growthTestSeries(1, 0, 1, 2), unmeasured, "hit rate is unknown"},
		"no memory reading": {growthTestSeries(1, 0, 1, 2), noMemory, "no connection's memory"},
	} {
		t.Run(name, func(t *testing.T) {
			failures := growthFailures([]growthSeries{tc.series}, tc.other, growthTestLimits, growthTestAssume)
			if len(failures) != 1 || !strings.Contains(failures[0], tc.want) {
				t.Fatalf("want one failure naming %q, got %v", tc.want, failures)
			}
		})
	}
}

func TestTheColdP90IsTheNearestRank(t *testing.T) {
	var samples []growthSample
	for ms := 10; ms >= 1; ms-- {
		samples = append(samples, growthSample{Elapsed: time.Duration(ms) * time.Millisecond})
	}
	if got := growthP90(samples); got != 9*time.Millisecond {
		t.Fatalf("p90 of 1..10 ms = %s, want 9ms", got)
	}
}
