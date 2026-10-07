// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The daily-use budgets (p95, per seat). PERF-1 and PERF-7 are declared beside
// their own benches; these are the screens only bench-daily measures. LOAD-1
// is the cheap route while every seat starts at once, judged on its own ID.
const (
	Perf2Budget  = 150 * time.Millisecond
	Perf8Budget  = time.Second
	Perf9Budget  = 300 * time.Millisecond
	Perf10Budget = time.Second
	Load1Budget  = 150 * time.Millisecond
)

// DailyVerdict is the recorded outcome of one flow for one seat; gen-perfdoc
// renders these exact strings, so a change here changes the published page.
type DailyVerdict string

const (
	DailyWithin    DailyVerdict = "within budget"
	DailyOverKnown DailyVerdict = "over budget"
	DailyOver      DailyVerdict = "over budget (unlisted)"
	DailyNoData    DailyVerdict = "no data"
	DailyNotGated  DailyVerdict = "not gated"
)

// KnownIssue ties a flow already over budget to the open issue tracking it.
type KnownIssue struct {
	Flow  string
	Issue int
}

// DailyKnownIssues lists flows already over budget, each tracked by an open
// issue; remove a row when its issue closes.
var DailyKnownIssues = []KnownIssue{
	{Flow: "worklist", Issue: 4912},
	{Flow: "palette_search_prefix", Issue: 7037},
	{Flow: "morning_load", Issue: 7068},
}

// DailyResult is one flow measured for one seat, as the gate judges it.
type DailyResult struct {
	Flow, Seat string
	Verdict    DailyVerdict
	Issue      int
	Status5xx  int
	Status422  int
	// Allow422 marks a flow whose query_too_broad refusal is the known issue itself.
	Allow422 bool
}

const dailyScaleEnv = "MARGINCE_BENCH_DAILY_SCALE"

func knownIssueFor(flow string) int {
	for _, known := range DailyKnownIssues {
		if known.Flow == flow {
			return known.Issue
		}
	}
	return 0
}

// JudgeDaily returns the verdict for one flow's p95 and, when it is over budget
// and listed, the issue that tracks it. A p95 equal to the budget is over, as
// gen-perfdoc renders it.
func JudgeDaily(flow string, p95, budget time.Duration, samples int) (DailyVerdict, int) {
	switch {
	case samples == 0:
		return DailyNoData, 0
	case p95 < budget:
		return DailyWithin, 0
	}
	if issue := knownIssueFor(flow); issue != 0 {
		return DailyOverKnown, issue
	}
	return DailyOver, 0
}

// DailyGate fails the run on every unlisted breach, every 5xx and every 422
// outside a listed flow. At the published scale (1) it also fails a listed flow
// now within budget for every measured seat, so the table cannot go stale.
func DailyGate(results []DailyResult, scale float64) error {
	var problems []string
	for _, r := range results {
		if r.Verdict == DailyOver {
			problems = append(problems, fmt.Sprintf("flow %s is over budget for %s and no open issue lists it", r.Flow, r.Seat))
		}
		if r.Status5xx > 0 {
			problems = append(problems, fmt.Sprintf("flow %s answered %d server errors for %s", r.Flow, r.Status5xx, r.Seat))
		}
		if r.Status422 > 0 && !r.Allow422 {
			problems = append(problems, fmt.Sprintf("flow %s was refused %d times with 422 for %s", r.Flow, r.Status422, r.Seat))
		}
	}
	if scale == 1 {
		problems = append(problems, staleKnownIssues(results)...)
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

// staleKnownIssues names each listed flow that was measured and came in within
// budget for every seat; a seat with no data or no gate says nothing either way.
// A row that drew a 422 was refused, not served fast, so it is never "within".
func staleKnownIssues(results []DailyResult) []string {
	var stale []string
	for _, known := range DailyKnownIssues {
		measured, within := 0, 0
		for _, r := range results {
			if r.Flow != known.Flow || r.Verdict == DailyNoData || r.Verdict == DailyNotGated {
				continue
			}
			measured++
			if r.Verdict == DailyWithin && r.Status422 == 0 {
				within++
			}
		}
		if measured > 0 && within == measured {
			stale = append(stale, fmt.Sprintf("flow %s is within budget for every seat: remove the row for #%d", known.Flow, known.Issue))
		}
	}
	return stale
}

// applySampleFloor turns every gated row with fewer than dailySamples samples
// into "no data": a p95 of a handful of tries is a guess, never a verdict.
func applySampleFloor(rows []dailyRow) []dailyRow {
	for i := range rows {
		r := &rows[i]
		if r.Result.Verdict == DailyNoData || r.Result.Verdict == DailyNotGated || r.Measurement.Samples >= dailySamples {
			continue
		}
		r.Result.Verdict, r.Result.Issue = DailyNoData, 0
		r.Measurement.Verdict, r.Measurement.KnownIssue = string(DailyNoData), 0
		r.Measurement.Note = joinNotes(fmt.Sprintf("%d samples, under the floor of %d: no p95 is claimed",
			r.Measurement.Samples, dailySamples), r.Measurement.Note)
	}
	return rows
}

func joinNotes(notes ...string) string {
	kept := slices.DeleteFunc(notes, func(n string) bool { return n == "" })
	return strings.Join(kept, "; ")
}

// dailyScale reads the corpus multiplier; 1 is the published mid-market tier.
// A value that is not a positive number is an error, never a silent default.
func dailyScale() (float64, error) {
	raw := os.Getenv(dailyScaleEnv)
	if raw == "" {
		return 1, nil
	}
	scale, err := strconv.ParseFloat(raw, 64)
	if err != nil || scale <= 0 {
		return 0, fmt.Errorf("%s=%q: set a positive number (1 for the published tier, 0.05 for a development run)", dailyScaleEnv, raw)
	}
	return scale, nil
}

func TestADailyFlowWithinItsBudgetPasses(t *testing.T) {
	v, issue := JudgeDaily("lists", 90*time.Millisecond, Perf2Budget, 30)
	if v != DailyWithin || issue != 0 {
		t.Fatalf("got %q #%d, want within budget and no issue", v, issue)
	}
}

func TestAKnownSlowFlowIsRecordedAgainstItsIssueAndPasses(t *testing.T) {
	v, issue := JudgeDaily("worklist", 3*time.Second, Perf8Budget, 30)
	if v != DailyOverKnown || issue != 4912 {
		t.Fatalf("got %q #%d, want over budget against #4912", v, issue)
	}
	if err := DailyGate([]DailyResult{{Flow: "worklist", Seat: "rep", Verdict: v, Issue: issue}}, 1); err != nil {
		t.Fatalf("a listed over-budget flow must not fail the run: %v", err)
	}
}

func TestAnUnlistedSlowFlowFailsTheRunNamingTheFlow(t *testing.T) {
	v, _ := JudgeDaily("record_open", 400*time.Millisecond, 100*time.Millisecond, 30)
	err := DailyGate([]DailyResult{{Flow: "record_open", Seat: "manager", Verdict: v}}, 1)
	if v != DailyOver || err == nil || !strings.Contains(err.Error(), "record_open") {
		t.Fatalf("got %q, err %v; want an unlisted breach that fails and names the flow", v, err)
	}
}

func TestAFlowWithNoSamplesIsNoDataNotAPass(t *testing.T) {
	if v, _ := JudgeDaily("home_digest", 0, Perf8Budget, 0); v != DailyNoData {
		t.Fatalf("got %q, want no data", v)
	}
}

func TestAServerErrorFailsEvenWithinBudget(t *testing.T) {
	err := DailyGate([]DailyResult{{Flow: "contact_360", Seat: "rep", Verdict: DailyWithin, Status5xx: 1}}, 1)
	if err == nil || !strings.Contains(err.Error(), "contact_360") {
		t.Fatalf("got %v; a 5xx must fail the run and name the flow", err)
	}
}

func TestA422IsAllowedOnlyWhereListed(t *testing.T) {
	ok := DailyGate([]DailyResult{{Flow: "palette_search_prefix", Seat: "rep", Verdict: DailyOverKnown, Issue: 7037, Status422: 3, Allow422: true}}, 1)
	bad := DailyGate([]DailyResult{{Flow: "results_search", Seat: "rep", Verdict: DailyWithin, Status422: 1}}, 1)
	if ok != nil || bad == nil || !strings.Contains(bad.Error(), "results_search") {
		t.Fatalf("listed 422 err=%v (want nil); unlisted 422 err=%v (want an error naming the flow)", ok, bad)
	}
}

func TestAListedFlowBackUnderBudgetForEverySeatAsksForItsRowToGo(t *testing.T) {
	err := DailyGate([]DailyResult{
		{Flow: "worklist", Seat: "rep", Verdict: DailyWithin},
		{Flow: "worklist", Seat: "manager", Verdict: DailyWithin},
		{Flow: "worklist", Seat: "cold", Verdict: DailyNotGated},
	}, 1)
	want := "flow worklist is within budget for every seat: remove the row for #4912"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v; want %q", err, want)
	}
}

func TestAListedFlowStillOverForOneSeatKeepsItsRow(t *testing.T) {
	err := DailyGate([]DailyResult{
		{Flow: "worklist", Seat: "rep", Verdict: DailyWithin},
		{Flow: "worklist", Seat: "manager", Verdict: DailyOverKnown, Issue: 4912},
	}, 1)
	if err != nil {
		t.Fatalf("a flow still over for the manager must keep its row: %v", err)
	}
}

func TestAListedFlowWithNoMeasuredSeatKeepsItsRow(t *testing.T) {
	if err := DailyGate([]DailyResult{{Flow: "worklist", Seat: "rep", Verdict: DailyNoData}}, 1); err != nil {
		t.Fatalf("a flow nobody measured says nothing about its issue: %v", err)
	}
}

func TestADevelopmentScaleRunNeverAsksForARowToGo(t *testing.T) {
	err := DailyGate([]DailyResult{{Flow: "worklist", Seat: "rep", Verdict: DailyWithin}}, 0.05)
	if err != nil {
		t.Fatalf("a development-scale corpus cannot clear a known issue: %v", err)
	}
}

func TestTheDailyScaleDefaultsToThePublishedTier(t *testing.T) {
	t.Setenv(dailyScaleEnv, "")
	if scale, err := dailyScale(); err != nil || scale != 1 {
		t.Fatalf("got %v, %v; want 1 and no error", scale, err)
	}
	t.Setenv(dailyScaleEnv, "0.05")
	if scale, err := dailyScale(); err != nil || scale != 0.05 {
		t.Fatalf("got %v, %v; want 0.05 and no error", scale, err)
	}
}

func TestTheDailyScaleRefusesAValueThatSeedsNothing(t *testing.T) {
	for _, raw := range []string{"0", "-1", "half"} {
		t.Setenv(dailyScaleEnv, raw)
		if _, err := dailyScale(); err == nil || !strings.Contains(err.Error(), dailyScaleEnv) {
			t.Fatalf("%s=%q: got %v; want an error naming the variable", dailyScaleEnv, raw, err)
		}
	}
}

func TestTheMorningLoadIsJudgedOnItsOwnBudgetAgainstItsIssue(t *testing.T) {
	v, issue := JudgeDaily("morning_load", 700*time.Millisecond, Load1Budget, 40)
	if v != DailyOverKnown || issue != 7068 {
		t.Fatalf("got %q #%d, want over budget against #7068", v, issue)
	}
}

func TestARefusedRowNeverCountsAsBackUnderBudget(t *testing.T) {
	err := DailyGate([]DailyResult{
		{Flow: "palette_search_prefix", Seat: "rep", Verdict: DailyWithin, Status422: 4, Allow422: true},
		{Flow: "palette_search_prefix", Seat: "manager", Verdict: DailyWithin},
	}, 1)
	if err != nil {
		t.Fatalf("a 422 is a refusal, not a fast answer; the row for #7037 must stay: %v", err)
	}
}

func TestARowUnderTheSampleFloorIsNoDataNeverAVerdict(t *testing.T) {
	short := MeasurementFrom("LOAD-1", "morning_load_cheap_route", 0, 700*time.Millisecond, 0, Load1Budget, 14)
	short.Verdict, short.KnownIssue = string(DailyOverKnown), 7068
	rows := applySampleFloor([]dailyRow{
		{Measurement: short, Result: DailyResult{Flow: "morning_load", Seat: "team", Verdict: DailyOverKnown, Issue: 7068, Status5xx: 1}},
		{Measurement: BudgetMeasurement{Samples: 30, Verdict: string(DailyWithin)}, Result: DailyResult{Verdict: DailyWithin}},
		{Measurement: BudgetMeasurement{Samples: 1, Verdict: string(DailyNotGated)}, Result: DailyResult{Verdict: DailyNotGated}},
	})
	floored := rows[0]
	if floored.Result.Verdict != DailyNoData || floored.Measurement.Verdict != string(DailyNoData) || floored.Measurement.KnownIssue != 0 {
		t.Fatalf("14 samples judged %q (record %q, #%d); want no data", floored.Result.Verdict, floored.Measurement.Verdict, floored.Measurement.KnownIssue)
	}
	if !strings.Contains(floored.Measurement.Note, "14 samples, under the floor of 30") || floored.Result.Status5xx != 1 {
		t.Fatalf("note %q, 5xx %d; want the floor named and the server error kept for the gate", floored.Measurement.Note, floored.Result.Status5xx)
	}
	if rows[1].Result.Verdict != DailyWithin || rows[2].Result.Verdict != DailyNotGated {
		t.Fatalf("a full row and an ungated one must keep their verdicts: %q, %q", rows[1].Result.Verdict, rows[2].Result.Verdict)
	}
}

func TestACorpusWhoseMedianRepOwnsNoProjectIsRefused(t *testing.T) {
	if missing := missingMedianKinds([]string{"deal", "company", "contact"}); !slices.Equal(missing, []string{"project"}) {
		t.Fatalf("got %v; the flows open the median rep's own project, so a corpus without one must fail the pick", missing)
	}
	if missing := missingMedianKinds(dailyMedianKinds); len(missing) != 0 {
		t.Fatalf("got %v; a rep owning every kind misses nothing", missing)
	}
}

func TestOnlyAnOptionalCallsNotFoundIsNoData(t *testing.T) {
	logo := dailyCall{Name: "logo", Path: "/v1/companies/1/logo", Optional: true}
	if untimed, err := probeAbsent(logo, http.StatusNotFound, nil); !untimed || err != nil {
		t.Fatalf("an optional 404 → untimed %v, err %v; want no data", untimed, err)
	}
	record := dailyCall{Name: "deal", Path: "/v1/deals/1"}
	if _, err := probeAbsent(record, http.StatusNotFound, nil); err == nil || !strings.Contains(err.Error(), "/v1/deals/1") {
		t.Fatalf("got %v; a 404 on a corpus-backed read must fail the run naming the path", err)
	}
}

func TestOnlyAnAllowlistedRouteMayAnswerNotImplemented(t *testing.T) {
	unwired := []byte(`{"code":"not_implemented","message":"not wired"}`)
	for path, want := range map[string]bool{
		"/v1/digest":                true,
		"/v1/connectors":            true,
		"/v1/digest?since=1":        true,
		"/v1/deals/1":               false,
		"/v1/analytics/framework":   false,
		"/v1/connectors/1/messages": false,
	} {
		if got := unwiredAnswer(path, http.StatusNotImplemented, unwired); got != want {
			t.Errorf("unwiredAnswer(%s) = %v, want %v", path, got, want)
		}
		untimed, err := probeAbsent(dailyCall{Path: path}, http.StatusNotImplemented, unwired)
		if untimed != want || err != nil {
			t.Errorf("probeAbsent(%s, 501) = %v, %v; want %v and no error", path, untimed, err, want)
		}
	}
}

func TestOnlyTimedSamplesAreTallied(t *testing.T) {
	failing := dailyFlow{Name: "contact_360"}
	var tally dailyTally
	_, err := dailySampling(1, func(counted bool) ([]time.Duration, error) {
		r := dailyResponse{dailyCall{Path: "/v1/contacts/1/360"}, http.StatusInternalServerError, nil}
		return []time.Duration{time.Millisecond}, failing.admit(r, tallied(&tally, counted))
	})
	if err != nil || tally.s5xx != dailySamples {
		t.Fatalf("err %v, %d server errors tallied; want %d, one per sample and none from the %d warm-ups", err, tally.s5xx, dailySamples, dailyWarmups)
	}
}

func TestTheProbeIsNeverTallied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	seat := Seat{Name: "rep-a1", Role: "rep", Client: srv.Client()}
	call := dailyCall{Path: "/v1/contacts/1/360"}
	timed, _, tally := probeCalls(t, &apptest.AppEnv{TS: srv}, dailyFlow{Name: "contact_360"}, seat, []dailyCall{call})
	if len(timed) != 1 || tally.s5xx != 0 {
		t.Fatalf("timed %v, %d server errors tallied; the probe checks an answer and never counts it", timed, tally.s5xx)
	}
}
