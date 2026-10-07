// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// TestDailyUseBudgets times the screens a rep and a manager open every day,
// over HTTP, as those seats. Its subtests share state and run in order (seats,
// seed, first_load, latency, load), so run it whole; a broken step stops the run.
func TestDailyUseBudgets(t *testing.T) {
	scale, err := dailyScale()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("daily bench at scale %g", scale)
	e := apptest.SetupApp(t)
	// Without a statement cache nothing is prepared, so the generic-plan
	// regressions this bench exists to catch could never appear in a sample.
	if mode := e.Pool.Config().ConnConfig.DefaultQueryExecMode; mode != pgx.QueryExecModeCacheStatement {
		t.Fatalf("the app pool prepares statements in mode %q, want cache_statement: run through make bench-daily, which sets default_query_exec_mode=cache_statement", mode)
	}
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)

	var seats Seats
	var corpus DailyCorpus
	var rows []dailyRow
	step := func(name string, run func(t *testing.T)) {
		if !t.Run(name, run) {
			t.FailNow()
		}
	}
	step("seats", func(t *testing.T) {
		seats = createDailySeats(t, e)
		if len(seats.Managers) != 2 || len(seats.Reps) != 2*dailyRepsPerTeam {
			t.Fatalf("signed in %d managers and %d reps, want 2 and %d", len(seats.Managers), len(seats.Reps), 2*dailyRepsPerTeam)
		}
		t.Logf("seats: %d signed in (%d managers, %d reps)", len(seats.Managers)+len(seats.Reps), len(seats.Managers), len(seats.Reps))
	})

	step("seed", func(t *testing.T) {
		start := time.Now()
		corpus = seedDailyCorpus(t, e, seats, scale)
		t.Logf("seed: corpus at scale %g in %s; flows open deals %q and companies %q",
			scale, time.Since(start).Round(time.Second), corpus.DealNames, corpus.CompanyNames)
		assertDailyContactScope(t, e, seats, corpus)
	})

	step("first_load", func(t *testing.T) {
		worklist, palette := runFirstLoad(t, e, seats, corpus)
		rows = append(rows, firstLoadRows(t, worklist, palette)...)
	})

	step("latency", func(t *testing.T) {
		rep := seats.Reps[slices.IndexFunc(seats.Reps, func(s Seat) bool { return s.UserID == corpus.MedianRepID })]
		for _, seat := range []Seat{rep, seats.Managers[0]} {
			in := newDailyInput(t, e, seat, seats.Teams[0], corpus)
			in.EvaluateArgs = dailyEvaluateArgs(t, e, seat)
			for _, f := range dailyFlows {
				rows = append(rows, runDailyFlow(t, e, f, in)...)
			}
		}
		assertDailyReviewFocus(t, rows)
	})

	step("load", func(t *testing.T) {
		absent := map[string]bool{}
		for _, row := range rows {
			if row.Result.Verdict == DailyNoData {
				absent[row.Measurement.Name] = true
			}
		}
		m := runMorningLoad(t, e, seats, corpus, absent)
		t.Logf("perfbench [daily]: morning_load team cheap p50=%s p95=%s samples=%d 5xx=%d 422=%d pool wait %s total, %s worst mean, %d acquires on a pool of %d",
			m.CheapP50, m.CheapP95, m.CheapSamples, m.S5xx, m.S422, m.PoolWaitTotal, m.PoolWaitMax, m.Acquires, e.Pool.Config().MaxConns)
		rows = append(rows, morningLoadRow(m))
	})

	// The record is written before the gate: a breach is the run a reader most needs to see.
	measurements := make([]BudgetMeasurement, 0, len(rows))
	results := make([]DailyResult, 0, len(rows))
	for _, row := range rows {
		if scale != 1 {
			row.Measurement.Caveat = strings.TrimSuffix(fmt.Sprintf("development scale %g; %s", scale, row.Measurement.Caveat), "; ")
		}
		measurements = append(measurements, row.Measurement)
		results = append(results, row.Result)
	}
	WritePerfRecord(t, "bench-daily", benchPostgresVersion(e.Owner), measurements)
	if err := DailyGate(results, scale); err != nil {
		t.Fatalf("daily-use budget gate is red: %v", err)
	}
}

// assertDailyReviewFocus holds the run to what makes its numbers mean
// anything: enough samples, a seat that sees data, and no flow timed empty.
func assertDailyReviewFocus(t *testing.T, rows []dailyRow) {
	t.Helper()
	answered, timedEmpty, sample := map[string]bool{}, map[string]bool{}, map[string][]byte{}
	for _, row := range rows {
		m := row.Measurement
		key := m.Flow + " for the " + m.Seat
		if gated := row.Result.Verdict != DailyNoData && row.Result.Verdict != DailyNotGated; gated && m.Samples < dailySamples {
			t.Errorf("%s (%s) took %d samples, want at least %d", m.Name, m.Seat, m.Samples, dailySamples)
		}
		answered[key] = answered[key] || !row.Empty
		timedEmpty[key] = timedEmpty[key] || row.Empty && row.Result.Verdict != DailyNoData
		if sample[key] == nil {
			sample[key] = row.Answer
		}
		if m.Name == "palette_search_company" && m.Seat == "rep" && !answerHasType(row.Answer, "company") {
			t.Errorf("the rep's palette search for a company name found no company: %s", clipBody(row.Answer))
		}
	}
	for key, empty := range timedEmpty {
		if empty && !answered[key] {
			t.Errorf("flow %s answered nothing but empty lists or 404s, so the seat sees no data; first answer: %s", key, clipBody(sample[key]))
		}
	}
}

func answerHasType(body []byte, kind string) bool {
	var answer struct {
		Data []struct {
			Type string `json:"type"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return false
	}
	return slices.ContainsFunc(answer.Data, func(hit struct {
		Type string `json:"type"`
	},
	) bool {
		return hit.Type == kind
	})
}
