// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"math"
	"slices"
	"time"
)

// What a reader of a run's progress may be told about its speed. The window is
// folded into the progress at every flush. A read computes the estimate from the
// row alone, and nothing in the worker is asked.
const (
	// windowSamples and windowSpan bound the samples one run keeps: the speed
	// of the last minute, not of the whole run.
	windowSamples = 16
	windowSpan    = 60 * time.Second
	// estimateMinSpan and estimateMinRows are the evidence below which no
	// estimate is given: a rate over two seconds of a cold start is a guess.
	estimateMinSpan = 10 * time.Second
	estimateMinRows = 200
	// estimateIdleAfter is how long without a flush a run is called idle. An
	// idle run has no speed to quote.
	estimateIdleAfter = 30 * time.Second
	// The bracket is the slowest and fastest rate the window shows, widened by
	// what a rate over a minute cannot see.
	estimateLowMargin  = 0.7
	estimateHighMargin = 1.4
)

// What an ETA covers. Until the run is on its last item, only the item it is on
// has a rate worth quoting. The next one is a different file.
const (
	EstimateScopeObject = "object"
	EstimateScopeRun    = "run"
)

// Sample is the rows a run had done at one instant of the database clock.
type Sample struct {
	At   time.Time `json:"at"`
	Done int       `json:"done"`
}

// Estimate is a run's speed and the range its current item finishes in.
type Estimate struct {
	RowsPerSecond float64
	LowSeconds    int
	HighSeconds   int
	Scope         string
}

// rowsDone counts the rows the run has walked, links included.
func (p Progress) rowsDone() int {
	done := 0
	for _, o := range p.Objects {
		done += o.RowsDone
	}
	return done
}

// foldWindow carries the previous window forward with one more sample, taken
// at `at` on the database clock. A window that ran backwards is another pass
// over the rows, so it starts over and a rate never spans two passes.
func (p *Progress) foldWindow(prior Progress, at time.Time) {
	done := p.rowsDone()
	window := prior.Window
	if n := len(window); n > 0 && (window[n-1].Done > done || prior.Attempt != p.Attempt) {
		window = nil
	}
	window = append(slices.Clip(window), Sample{At: at, Done: done})
	for len(window) > 2 && at.Sub(window[0].At) > windowSpan {
		window = window[1:]
	}
	if len(window) > windowSamples {
		window = window[len(window)-windowSamples:]
	}
	p.Window = window
}

// EstimateOf answers the run's speed and the range its current item finishes
// in, at readAt on the database clock. It answers false when it has too little
// to go on, when the run has stopped moving, or when no item is left.
func EstimateOf(p Progress, progressAt, readAt time.Time) (Estimate, bool) {
	w := p.Window
	if len(w) < 2 || progressAt.IsZero() || readAt.Sub(progressAt) > estimateIdleAfter {
		return Estimate{}, false
	}
	first, last := w[0], w[len(w)-1]
	if last.At.Sub(first.At) < estimateMinSpan || last.Done-first.Done < estimateMinRows {
		return Estimate{}, false
	}
	remaining, scope := remainingOfCurrentItem(p)
	if remaining <= 0 {
		return Estimate{}, false
	}
	whole := rateOf(first, last)
	slow, fast := whole, whole
	mid := w[len(w)/2]
	for _, half := range [][2]Sample{{first, mid}, {mid, last}} {
		if r := rateOf(half[0], half[1]); r > 0 {
			slow, fast = math.Min(slow, r), math.Max(fast, r)
		}
	}
	if whole <= 0 {
		return Estimate{}, false
	}
	sinceSample := readAt.Sub(last.At).Seconds()
	low := float64(remaining)/fast*estimateLowMargin - sinceSample
	high := float64(remaining)/slow*estimateHighMargin - sinceSample
	est := Estimate{RowsPerSecond: whole, Scope: scope}
	est.LowSeconds = max(1, int(math.Floor(low)))
	est.HighSeconds = max(est.LowSeconds, int(math.Ceil(high)))
	return est, true
}

func rateOf(from, to Sample) float64 {
	span := to.At.Sub(from.At).Seconds()
	if span <= 0 || to.Done <= from.Done {
		return 0
	}
	return float64(to.Done-from.Done) / span
}

// remainingOfCurrentItem is the rows left in the first item not yet done, and
// whether that item is the last one.
func remainingOfCurrentItem(p Progress) (int, string) {
	for i, o := range p.Objects {
		if o.RowsDone < o.RowsTotal {
			scope := EstimateScopeObject
			if i == len(p.Objects)-1 {
				scope = EstimateScopeRun
			}
			return o.RowsTotal - o.RowsDone, scope
		}
	}
	return 0, ""
}

// Phases answers the phases a run reports, in a new slice. The contract's
// ImportRunPhase enum is this same list, and a test holds the two equal.
func Phases() []string {
	return []string{
		PhaseReading, PhaseChecking, PhaseCreatingTags, PhaseImporting,
		PhaseLinking, PhaseUndoing, PhaseFinishing,
	}
}
