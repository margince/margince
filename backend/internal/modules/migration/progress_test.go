// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"math"
	"testing"
	"time"
)

var epoch = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)

func at(seconds float64) time.Time {
	return epoch.Add(time.Duration(seconds * float64(time.Second)))
}

// oneObject is a run over one file of total rows, done of them walked.
func oneObject(done, total int, window ...Sample) Progress {
	return Progress{
		Objects: []ObjectProgress{{Object: "contact", RowsDone: done, RowsTotal: total}},
		Window:  window,
	}
}

func TestEstimateIsAbsentUntilEnoughRowsAndTime(t *testing.T) {
	cases := map[string]Progress{
		"one sample":      oneObject(500, 6000, Sample{at(0), 500}),
		"under ten s":     oneObject(900, 6000, Sample{at(0), 100}, Sample{at(8), 900}),
		"under 200 rows":  oneObject(250, 6000, Sample{at(0), 100}, Sample{at(20), 250}),
		"no rows moved":   oneObject(100, 6000, Sample{at(0), 100}, Sample{at(30), 100}),
		"nothing is left": oneObject(6000, 6000, Sample{at(0), 100}, Sample{at(30), 6000}),
	}
	for name, p := range cases {
		if est, ok := EstimateOf(p, at(30), at(30)); ok {
			t.Errorf("%s: estimate = %+v, want none", name, est)
		}
	}
	enough := oneObject(1100, 6000, Sample{at(0), 100}, Sample{at(20), 1100})
	if est, ok := EstimateOf(enough, at(20), at(20)); !ok || math.Abs(est.RowsPerSecond-50) > 0.001 {
		t.Fatalf("estimate = %+v, %v, want 50 rows/s once 10 s and 200 rows are seen", est, ok)
	}
}

func TestEstimateIsAbsentWhileIdle(t *testing.T) {
	p := oneObject(1100, 6000, Sample{at(0), 100}, Sample{at(20), 1100})
	if _, ok := EstimateOf(p, at(20), at(20+estimateIdleAfter.Seconds())); !ok {
		t.Fatal("a run flushed 30 s ago is not yet idle")
	}
	if est, ok := EstimateOf(p, at(20), at(51)); ok {
		t.Fatalf("estimate = %+v after 31 s with no flush, want none", est)
	}
}

func TestEstimateScopeIsObjectUntilTheLastItem(t *testing.T) {
	window := []Sample{{at(0), 0}, {at(20), 1000}}
	p := Progress{
		Objects: []ObjectProgress{
			{Object: "company", RowsDone: 1000, RowsTotal: 5000},
			{Object: "contact", RowsDone: 0, RowsTotal: 5000},
			{Object: progressLinks, RowsDone: 0, RowsTotal: 3000},
		},
		Window: window,
	}
	est, ok := EstimateOf(p, at(20), at(20))
	if !ok || est.Scope != EstimateScopeObject {
		t.Fatalf("estimate = %+v, %v, want object scope on the first of three", est, ok)
	}
	p.Objects[0].RowsDone, p.Objects[1].RowsDone = 5000, 5000
	est, ok = EstimateOf(p, at(20), at(20))
	if !ok || est.Scope != EstimateScopeRun {
		t.Fatalf("estimate = %+v, %v, want run scope once on the links, the last item", est, ok)
	}
}

// A run whose speed drifts by a third either way is still bracketed most of the
// time. The low end is never zero before the end.
func TestTheEstimateBracketsAVaryingRateRun(t *testing.T) {
	const total, flushEvery = 12000, 2.0
	rate := func(s float64) float64 { return 60 + 20*math.Sin(s/25) }
	var done float64
	var finish float64
	type point struct {
		at   float64
		done int
	}
	var trace []point
	for s := 0.0; done < total; s += 0.5 {
		done = math.Min(total, done+rate(s)*0.5)
		finish = s + 0.5
		if math.Mod(s+0.5, flushEvery) == 0 || done >= total {
			trace = append(trace, point{s + 0.5, int(done)})
		}
	}
	var window []Sample
	var bracketed, quoted int
	for _, pt := range trace[:len(trace)-1] {
		p := oneObject(pt.done, total)
		p.Attempt = 1
		prior := Progress{Attempt: 1, Window: window}
		p.foldWindow(prior, at(pt.at))
		window = p.Window
		est, ok := EstimateOf(p, at(pt.at), at(pt.at))
		if !ok {
			continue
		}
		quoted++
		if est.LowSeconds < 1 {
			t.Fatalf("t=%.0f: low end %d, want at least 1 before the end", pt.at, est.LowSeconds)
		}
		left := finish - pt.at
		if float64(est.LowSeconds) <= left && left <= float64(est.HighSeconds) {
			bracketed++
		}
	}
	if quoted < 20 {
		t.Fatalf("only %d estimates over the run, want one per flush after the first ten seconds", quoted)
	}
	if share := float64(bracketed) / float64(quoted); share < 0.8 {
		t.Fatalf("%d of %d estimates bracketed the real finish (%.0f%%), want at least 80%%", bracketed, quoted, share*100)
	}
}

func TestWindowKeepsAMinuteAndStartsOverOnANewPass(t *testing.T) {
	var p Progress
	prior := Progress{}
	for s := 0; s <= 200; s += 2 {
		p = oneObject(s*10, 5000)
		p.foldWindow(prior, at(float64(s)))
		prior = p
	}
	if n := len(p.Window); n > windowSamples {
		t.Fatalf("window holds %d samples, want at most %d", n, windowSamples)
	}
	if span := p.Window[len(p.Window)-1].At.Sub(p.Window[0].At); span > windowSpan {
		t.Fatalf("window spans %s, want at most %s", span, windowSpan)
	}
	again := oneObject(0, 5000)
	again.foldWindow(prior, at(210))
	if len(again.Window) != 1 || again.Window[0].Done != 0 {
		t.Fatalf("window after the rows went backwards = %+v, want it to start over", again.Window)
	}
}

// Companies walk at 500 rows/s and contacts at 30. Once the run is on the
// contacts, the estimate quotes the contacts' speed and not a blend with the
// companies that came before.
func TestTheEstimateAcrossAnObjectBoundaryQuotesTheNewObjectsRate(t *testing.T) {
	const companies, contacts = 20000, 6000
	var prior Progress
	var last Progress
	fold := func(seconds, companyDone, contactDone int) {
		last = Progress{Objects: []ObjectProgress{
			{Object: "company", RowsDone: companyDone, RowsTotal: companies},
			{Object: "contact", RowsDone: contactDone, RowsTotal: contacts},
		}}
		last.foldWindow(prior, at(float64(seconds)))
		prior = last
	}
	for s := 0; s <= 40; s += 2 {
		fold(s, min(s*500, companies), 0)
	}
	for s := 42; s <= 56; s += 2 {
		fold(s, companies, (s-40)*30)
	}
	est, ok := EstimateOf(last, at(56), at(56))
	if !ok {
		t.Fatal("no estimate 16 s into the contacts, want one from the contacts' own samples")
	}
	if math.Abs(est.RowsPerSecond-30) > 1 {
		t.Fatalf("rate = %.1f rows/s, want the contacts' 30 and not a blend with the companies' 500", est.RowsPerSecond)
	}
	if want := (contacts - 16*30) / 30; est.HighSeconds < want {
		t.Fatalf("high end %d s, want at least the %d s the contacts need", est.HighSeconds, want)
	}
}
