// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/search"
)

// Three warm-ups, then thirty samples on the seat's long-lived client: every
// statement runs past its fifth execution, where a generic plan takes over.
const (
	dailyWarmups = 3
	dailySamples = 30
)

// dailyEmptyNote marks a row timed over an answer with nothing in it, which
// says how fast an empty list is and nothing about the screen with data.
const dailyEmptyNote = "empty on the seeded corpus"

// dailyCall is one request a screen sends; Name tells its row apart from the
// flow's others. Optional marks a read the seeded corpus has nothing behind
// (a brief, a review, a logo), whose 404 is "no data"; anywhere else a 404 means
// the bench asked for something the corpus should hold, and fails the run.
type dailyCall struct {
	Name, Path string
	Optional   bool
}

// dailyFlow is one screen as the frontend calls it. PerCall times each call
// as its own row; a Journey is a search, then the page it opens, in parallel.
type dailyFlow struct {
	Name, ID string
	Budget   time.Duration
	Allow422 bool
	PerCall  bool
	Journey  bool
	Calls    func(in dailyInput) []dailyCall
}

// dailyRow is one recorded row and what the gate reads off it. Empty and
// Answer come from the first response, for the review-focus assertions.
type dailyRow struct {
	Measurement BudgetMeasurement
	Result      DailyResult
	Empty       bool
	Answer      []byte
}

// dailyInput is what one seat's flows open: the median rep's records, so the
// rep opens their own and the manager opens the same ones from above.
type dailyInput struct {
	Seat                                           Seat
	Team                                           string
	DealID, DealName, CompanyID, CompanyName       string
	ContactID, ProjectID, ListPrefix, EvaluateArgs string
}

var dailyFlows = []dailyFlow{
	{Name: "app_shell", ID: "PERF-2", Budget: Perf2Budget, PerCall: true, Calls: appShellCalls},
	{Name: "worklist", ID: "PERF-8", Budget: Perf8Budget, PerCall: true, Calls: worklistCalls},
	{Name: "home", ID: "PERF-8", Budget: Perf8Budget, PerCall: true, Calls: homeCalls},
	{Name: "palette_search", ID: "PERF-10", Budget: Perf10Budget, PerCall: true, Calls: paletteCalls},
	{Name: "palette_search_prefix", ID: "PERF-10", Budget: Perf10Budget, PerCall: true, Allow422: true, Calls: prefixCalls},
	{Name: "results_search", ID: "PERF-10", Budget: Perf10Budget, PerCall: true, Calls: resultsCalls},
	{Name: "lists", ID: "PERF-2", Budget: Perf2Budget, PerCall: true, Calls: listCalls},
	{Name: "record_open", ID: "PERF-1", Budget: perf1RecordOpenBudget, PerCall: true, Calls: recordOpenCalls},
	{Name: "contact_360", ID: "PERF-7", Budget: search.Perf7Budget, Calls: func(in dailyInput) []dailyCall {
		return []dailyCall{{Path: "/v1/contacts/" + in.ContactID + "/360"}}
	}},
	{Name: "search_to_deal", Journey: true, Calls: func(in dailyInput) []dailyCall {
		page := "/v1/deals/" + in.DealID
		return []dailyCall{
			paletteCall("search", in.DealName),
			{Name: "deal", Path: page},
			{Name: "offers", Path: page + "/offers"},
			{Name: "documents", Path: page + "/documents"},
			{Name: "coverage", Path: page + "/coverage"},
			{Name: "commitments", Path: page + "/commitments"},
		}
	}},
	{Name: "search_to_company", Journey: true, Calls: func(in dailyInput) []dailyCall {
		page := "/v1/companies/" + in.CompanyID
		return []dailyCall{
			paletteCall("search", in.CompanyName),
			{Name: "company", Path: page},
			{Name: "facts", Path: page + "/facts"},
			{Name: "documents", Path: page + "/documents"},
			{Name: "logo", Path: page + "/logo", Optional: true},
		}
	}},
	{Name: "analytics", ID: "PERF-9", Budget: Perf9Budget, PerCall: true, Calls: analyticsCalls},
}

// appShellCalls are the reads App.tsx, the shell and its agent rail send on load
// for a rep or a manager; reads the shell gates on an admin-only grant are left out.
func appShellCalls(dailyInput) []dailyCall {
	return []dailyCall{
		{Name: "capabilities", Path: "/v1/auth/capabilities"},
		{Name: "me", Path: "/v1/me"},
		{Name: "company_context", Path: "/v1/company/context/capabilities"},
		{Name: "installation_settings", Path: "/v1/installation/settings"},
		{Name: "connectors", Path: "/v1/connectors"},
		{Name: "approvals", Path: "/v1/approvals?status=pending&limit=50"},
		{Name: "assistant_profile", Path: "/v1/assistant/profile"},
		{Name: "ai_activity", Path: dailyAIActivityPath()},
		{Name: "notices", Path: "/v1/notices"},
	}
}

// dailyAIActivityPath is the agent rail's activity read: one kinds parameter per
// kind it narrates (displayedKinds in ai-activity-speak.ts), in the screen's order.
func dailyAIActivityPath() string {
	args := url.Values{"kinds": {
		"morning_brief", "overnight_at_risk_sweep", "document_extract", "site_read", "summarize", "account_scan",
		"draft_reply", "offer_draft", "weekly_review", "weekly_learnings", "transcript_propose", "voice_build",
	}}
	return "/v1/me/ai-activity?" + args.Encode()
}

// worklistCalls sends the scope the screen opens on: a rep's own queue, a manager's team.
func worklistCalls(in dailyInput) []dailyCall {
	scope := "mine"
	if in.Seat.Role == "manager" {
		scope = "team"
	}
	return []dailyCall{{Path: "/v1/worklist?scope=" + scope + "&filter=all"}, {Name: "handled", Path: "/v1/worklist/handled"}}
}

func homeCalls(in dailyInput) []dailyCall {
	calls := []dailyCall{
		{Name: "brief", Path: "/v1/brief", Optional: true},
		{Name: "worklist", Path: "/v1/worklist?scope=mine&filter=all"},
		{Name: "digest", Path: "/v1/digest", Optional: true},
		{Name: "weekly_reviews", Path: "/v1/weekly-reviews", Optional: true},
	}
	if in.Seat.Role == "manager" {
		calls = append(calls, dailyCall{Name: "weekly_reviews_team", Path: "/v1/weekly-reviews/team?team=" + in.Team, Optional: true})
	}
	return calls
}

// paletteCall is the command palette's request: three hits per type, colleagues included.
func paletteCall(name, q string) dailyCall {
	return dailyCall{Name: name, Path: "/v1/search?q=" + url.QueryEscape(q) + "&per_type=3&with_employees=true"}
}

func paletteCalls(in dailyInput) []dailyCall {
	return []dailyCall{
		paletteCall("contract", "contract"), paletteCall("angebot", "angebot"),
		paletteCall("meeting", "meeting"), paletteCall("company", in.CompanyName),
	}
}

func prefixCalls(dailyInput) []dailyCall {
	return []dailyCall{
		paletteCall("co", "co"), paletteCall("con", "con"),
		paletteCall("cont", "cont"), paletteCall("ang", "ang"),
	}
}

func resultsCalls(dailyInput) []dailyCall {
	return []dailyCall{
		{Name: "all", Path: "/v1/search?q=contract&per_type=5&with_employees=true"},
		{Name: "activity", Path: "/v1/search?q=contract&types=activity&limit=50&with_employees=true"},
	}
}

func listCalls(in dailyInput) []dailyCall {
	var calls []dailyCall
	for _, list := range []string{"companies", "contacts", "deals", "leads"} {
		calls = append(calls, dailyCall{Name: list, Path: "/v1/" + list + "?limit=50"},
			dailyCall{Name: list + "_q", Path: "/v1/" + list + "?q=" + url.QueryEscape(in.ListPrefix) + "&limit=50"})
	}
	return calls
}

func recordOpenCalls(in dailyInput) []dailyCall {
	return []dailyCall{
		{Name: "contact", Path: "/v1/contacts/" + in.ContactID},
		{Name: "company", Path: "/v1/companies/" + in.CompanyID},
		{Name: "deal", Path: "/v1/deals/" + in.DealID},
		{Name: "project", Path: "/v1/projects/" + in.ProjectID},
	}
}

// analyticsCalls are the five requests the analytics screen sends before it draws.
func analyticsCalls(in dailyInput) []dailyCall {
	return []dailyCall{
		{Name: "context", Path: "/v1/analytics/context"},
		{Name: "metrics", Path: "/v1/analytics/metrics"},
		{Name: "framework", Path: "/v1/analytics/framework"},
		{Name: "pipelines", Path: "/v1/pipelines"},
		{Name: "evaluate", Path: "/v1/analytics/evaluate?" + in.EvaluateArgs},
	}
}

func dailyRowName(flow string, call dailyCall) string {
	if call.Name == "" {
		return flow
	}
	return flow + "_" + call.Name
}

// dailyTally counts what a row's responses answered, for the record and the gate.
type dailyTally struct {
	s5xx, s422 int
	empty      bool
	// emptyCalls names the calls of a composite row that answered empty.
	emptyCalls []string
	answer     []byte
	// first5xx keeps the first server error's answer, the evidence a finding needs.
	first5xx string
}

// dailyResponse is one answered call of a round.
type dailyResponse struct {
	call   dailyCall
	status int
	body   []byte
}

// admit counts a 5xx or a listed 422 for the gate; any other refusal means
// the bench asked wrongly, so it fails the run naming the call.
func (f dailyFlow) admit(r dailyResponse, tally *dailyTally) error {
	switch {
	case r.status >= 200 && r.status < 300:
		return nil
	case r.status >= 500:
		if tally.s5xx++; tally.first5xx == "" {
			tally.first5xx = fmt.Sprintf("GET %s answered %d: %s", r.call.Path, r.status, clipBody(r.body))
		}
		return nil
	case r.status == http.StatusUnprocessableEntity && (!f.Allow422 || bytes.Contains(r.body, []byte("query_too_broad"))):
		tally.s422++
		return nil
	}
	return fmt.Errorf("GET %s answered %d: %s", r.call.Path, r.status, clipBody(r.body))
}

// tallied is the tally a round counts into: the row's own for a sample, a
// discarded one for a warm-up, whose answers are still checked.
func tallied(row *dailyTally, counted bool) *dailyTally {
	if counted {
		return row
	}
	return &dailyTally{}
}

func clipBody(body []byte) string {
	const keep = 400
	if len(body) > keep {
		return string(body[:keep]) + "…"
	}
	return string(body)
}

// fetchRound sends a round's calls in order, or all at once as a page does.
func fetchRound(s Seat, base string, calls []dailyCall, parallel bool) ([]dailyResponse, error) {
	out := make([]dailyResponse, len(calls))
	errs := make([]error, len(calls))
	fetch := func(i int) {
		status, body, _, err := s.fetch(context.Background(), base, calls[i].Path)
		out[i], errs[i] = dailyResponse{calls[i], status, body}, err
	}
	if !parallel {
		for i := range calls {
			if fetch(i); errs[i] != nil {
				return nil, errs[i]
			}
		}
		return out, nil
	}
	var wg sync.WaitGroup
	for i := range calls {
		wg.Go(func() { fetch(i) })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// dailyUnwiredNote names a no-data row whose operation this composition does
// not serve: the harness configures no mail connector, and these routes are
// wired only with one. Any other 501 is a server fault and fails the run.
const dailyUnwiredNote = "answers 501 until a mail connector is configured"

var dailyUnwiredRoutes = []string{"/v1/digest", "/v1/connectors"}

// unwiredAnswer is the generated 501 of a route on dailyUnwiredRoutes: the
// composition's posture, not a server fault, and nothing to time.
func unwiredAnswer(path string, status int, body []byte) bool {
	route, _, _ := strings.Cut(path, "?")
	return status == http.StatusNotImplemented && slices.Contains(dailyUnwiredRoutes, route) &&
		bytes.Contains(body, []byte(`"code":"not_implemented"`))
}

// dailyAbsent is a call with nothing to time, and the status that said so.
type dailyAbsent struct {
	call   dailyCall
	status int
}

// probeCalls asks each call once before timing. An optional call's 404 is the
// screen's own "nothing yet" and an unwired 501 has nothing behind it: either
// leaves the timed set as a no-data row. The probe's answers are not counted.
func probeCalls(t *testing.T, e *apptest.AppEnv, f dailyFlow, s Seat, calls []dailyCall) (timed []dailyCall, absent []dailyAbsent, tally dailyTally) {
	t.Helper()
	tally.empty = true
	for _, call := range calls {
		status, body, _ := s.Get(t, e, call.Path)
		untimed, err := probeAbsent(call, status, body)
		if err != nil {
			t.Fatalf("seat %s, flow %s: %v", s.Name, f.Name, err)
		}
		if untimed {
			absent = append(absent, dailyAbsent{call, status})
			continue
		}
		if err := f.admit(dailyResponse{call, status, body}, &dailyTally{}); err != nil {
			t.Fatalf("seat %s, flow %s: %v", s.Name, f.Name, err)
		}
		timed = append(timed, call)
		switch {
		case status >= 300:
		case emptyAnswer(body):
			tally.emptyCalls = append(tally.emptyCalls, call.Name)
		default:
			tally.empty = false
		}
		if tally.answer == nil {
			tally.answer = body
		}
	}
	return timed, absent, tally
}

// probeAbsent reports whether a probe's answer leaves its call untimed, and
// refuses a 404 on any call not marked Optional.
func probeAbsent(call dailyCall, status int, body []byte) (bool, error) {
	switch {
	case unwiredAnswer(call.Path, status, body), status == http.StatusNotFound && call.Optional:
		return true, nil
	case status == http.StatusNotFound:
		return false, fmt.Errorf("GET %s answered 404 on a read the seeded corpus backs: %s", call.Path, clipBody(body))
	}
	return false, nil
}

// emptyAnswer is an empty JSON list, or an object whose lists are all empty
// and which carries no record: a nested object counts, its page envelope does not.
func emptyAnswer(body []byte) bool {
	var list []json.RawMessage
	if json.Unmarshal(body, &list) == nil {
		return len(list) == 0
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return false
	}
	lists := 0
	for key, value := range object {
		var nested map[string]json.RawMessage
		switch {
		case json.Unmarshal(value, &list) == nil:
			if len(list) > 0 {
				return false
			}
			lists++
		case key != "page" && json.Unmarshal(value, &nested) == nil && nested != nil:
			return false
		}
	}
	return lists > 0 || len(object) == 0
}

// dailySampling times warm-ups and then samples of one round. A round returns
// one duration per row it times, which is why it is not benchRuns: a journey
// times its search, its page and the whole from one round. counted is false
// for a warm-up, whose answers are checked but never tallied.
func dailySampling(rows int, round func(counted bool) ([]time.Duration, error)) ([][]time.Duration, error) {
	for i := range dailyWarmups {
		if _, err := round(false); err != nil {
			return nil, fmt.Errorf("warm-up %d: %w", i+1, err)
		}
	}
	samples := make([][]time.Duration, rows)
	for i := range dailySamples {
		durations, err := round(true)
		if err != nil {
			return nil, fmt.Errorf("sample %d: %w", i+1, err)
		}
		for row, d := range durations {
			samples[row] = append(samples[row], d)
		}
	}
	return samples, nil
}

// runDailyFlow measures one flow for one seat and returns its rows.
func runDailyFlow(t *testing.T, e *apptest.AppEnv, f dailyFlow, in dailyInput) []dailyRow {
	t.Helper()
	if f.Journey {
		return runDailyJourney(t, e, f, in)
	}
	calls := f.Calls(in)
	if !f.PerCall {
		return measureDailyRow(t, e, f, in.Seat, f.Name, calls)
	}
	var rows []dailyRow
	for _, call := range calls {
		rows = append(rows, measureDailyRow(t, e, f, in.Seat, dailyRowName(f.Name, call), []dailyCall{call})...)
	}
	return rows
}

// measureDailyRow times calls in sequence as one row, after their no-data rows.
func measureDailyRow(t *testing.T, e *apptest.AppEnv, f dailyFlow, s Seat, name string, calls []dailyCall) []dailyRow {
	t.Helper()
	timed, absent, tally := probeCalls(t, e, f, s, calls)
	rows := dailyAbsentRows(t.Log, f, f.ID, name, s, f.Budget, absent, len(calls) == 1)
	if len(timed) == 0 {
		return rows
	}
	samples, err := dailySampling(1, func(counted bool) ([]time.Duration, error) {
		start := time.Now()
		answered, err := fetchRound(s, e.TS.URL, timed, false)
		elapsed := time.Since(start)
		if err != nil {
			return nil, err
		}
		for _, r := range answered {
			if err := f.admit(r, tallied(&tally, counted)); err != nil {
				return nil, err
			}
		}
		return []time.Duration{elapsed}, nil
	})
	if err != nil {
		t.Fatalf("seat %s, %s: %v", s.Name, name, err)
	}
	spec := dailyRowSpec{flow: f.Name, id: f.ID, name: name, budget: f.Budget, allow422: f.Allow422, gated: true}
	return append(rows, dailyMeasuredRow(t, s, spec, samples[0], tally))
}

// runDailyJourney times a palette search for a record, then that record's page
// as the browser loads it, in parallel. The halves answer to PERF-10 and
// PERF-1; the whole is recorded beside them, gated by neither.
func runDailyJourney(t *testing.T, e *apptest.AppEnv, f dailyFlow, in dailyInput) []dailyRow {
	t.Helper()
	s, calls := in.Seat, f.Calls(in)
	searchTimed, _, searchTally := probeCalls(t, e, f, s, calls[:1])
	if len(searchTimed) != 1 {
		t.Fatalf("seat %s, %s: the palette search answered 404", s.Name, f.Name)
	}
	page, absent, pageTally := probeCalls(t, e, f, s, calls[1:])
	pageNote := "the page's calls, in parallel: " + strings.Join(callNames(page), ", ")
	rows := dailyAbsentRows(t.Log, f, "PERF-1", f.Name+"_page", s, perf1RecordOpenBudget, absent, false)
	samples, err := dailySampling(3, func(counted bool) ([]time.Duration, error) {
		start := time.Now()
		found, err := fetchRound(s, e.TS.URL, calls[:1], false)
		searched := time.Now()
		if err != nil {
			return nil, err
		}
		opened, err := fetchRound(s, e.TS.URL, page, true)
		end := time.Now()
		if err != nil {
			return nil, err
		}
		for _, r := range found {
			if err := f.admit(r, tallied(&searchTally, counted)); err != nil {
				return nil, err
			}
		}
		for _, r := range opened {
			if err := f.admit(r, tallied(&pageTally, counted)); err != nil {
				return nil, err
			}
		}
		return []time.Duration{searched.Sub(start), end.Sub(searched), end.Sub(start)}, nil
	})
	if err != nil {
		t.Fatalf("seat %s, %s: %v", s.Name, f.Name, err)
	}
	return append(rows,
		dailyMeasuredRow(t, s, dailyRowSpec{flow: f.Name, id: "PERF-10", name: f.Name + "_search", budget: Perf10Budget, gated: true},
			samples[0], searchTally),
		dailyMeasuredRow(t, s, dailyRowSpec{flow: f.Name, id: "PERF-1", name: f.Name + "_page", budget: perf1RecordOpenBudget, gated: true, note: pageNote},
			samples[1], pageTally),
		dailyMeasuredRow(t, s, dailyRowSpec{flow: f.Name, id: "PERF-10", name: f.Name + "_total", budget: Perf10Budget + perf1RecordOpenBudget},
			samples[2], dailyTally{empty: searchTally.empty && pageTally.empty}))
}

// dailyAbsentRows records each call with nothing to time as its own no-data
// row, never averaged into the calls that answered.
func dailyAbsentRows(log func(...any), f dailyFlow, id, name string, s Seat, budget time.Duration, absent []dailyAbsent, alone bool) []dailyRow {
	rows := make([]dailyRow, 0, len(absent))
	for _, a := range absent {
		rowName := name
		if !alone {
			rowName = name + "_" + a.call.Name
		}
		m := MeasurementFrom(id, rowName, 0, 0, 0, budget, 0)
		m.Seat, m.Flow, m.Verdict = s.Role, f.Name, string(DailyNoData)
		m.Note = "answers 404: nothing of this kind on the seeded corpus"
		if a.status == http.StatusNotImplemented {
			m.Note = dailyUnwiredNote
		}
		log(fmt.Sprintf("perfbench [daily]: %s %s GET %s answered %d samples=0 %s", rowName, s.Role, a.call.Path, a.status, DailyNoData))
		rows = append(rows, dailyRow{
			Measurement: m,
			Result:      DailyResult{Flow: f.Name, Row: rowName, Seat: s.Role, Verdict: DailyNoData},
			Empty:       true,
		})
	}
	return rows
}

// dailyRowSpec names a row and how it is judged; an ungated row is recorded beside the budget, never against it.
type dailyRowSpec struct {
	flow, id, name, note string
	budget               time.Duration
	allow422, gated      bool
}

func callNames(calls []dailyCall) []string {
	names := make([]string, len(calls))
	for i, call := range calls {
		names[i] = call.Name
	}
	return names
}

// dailyMeasuredRow folds a row's samples into the record and the gate's
// result, and logs the line a reader of the run scans first.
func dailyMeasuredRow(t *testing.T, s Seat, spec dailyRowSpec, durations []time.Duration, tally dailyTally) dailyRow {
	t.Helper()
	flow, id, name, budget := spec.flow, spec.id, spec.name, spec.budget
	stats, err := search.MeasureQuery(name, budget, durations)
	if err != nil {
		t.Fatalf("seat %s, %s: %v", s.Name, name, err)
	}
	verdict, issue := DailyNotGated, 0
	if spec.gated {
		verdict, issue = JudgeDaily(flow, name, stats.P95, budget, stats.Samples)
	}
	m := MeasurementFrom(id, name, stats.P50, stats.P95, stats.P99, budget, stats.Samples)
	m.Seat, m.Flow, m.Verdict, m.KnownIssue = s.Role, flow, string(verdict), issue
	m.Status5xx, m.Status422 = tally.s5xx, tally.s422
	switch {
	case tally.empty:
		m.Note = joinNotes(spec.note, dailyEmptyNote)
	case len(tally.emptyCalls) > 0:
		m.Note = joinNotes(spec.note, strings.Join(tally.emptyCalls, ", ")+" "+dailyEmptyNote)
	default:
		m.Note = spec.note
	}
	if tally.first5xx != "" {
		t.Logf("perfbench [daily]: %s %s server error: %s", name, s.Role, tally.first5xx)
	}
	t.Logf("perfbench [daily]: %s %s p50=%s p95=%s p99=%s budget=%s samples=%d %s 5xx=%d 422=%d %s",
		name, s.Role, stats.P50, stats.P95, stats.P99, budget, stats.Samples, verdict, tally.s5xx, tally.s422, m.Note)
	return dailyRow{
		Measurement: m,
		Result: DailyResult{
			Flow: flow, Row: name, Seat: s.Role, Verdict: verdict, Issue: issue,
			Status5xx: tally.s5xx, Status422: tally.s422, Allow422: spec.allow422,
		},
		Empty:  tally.empty,
		Answer: tally.answer,
	}
}

// newDailyInput picks the median rep's own deal, company, contact and project
// from the corpus, so both seats open the same records.
func newDailyInput(t *testing.T, e *apptest.AppEnv, s Seat, team string, c DailyCorpus) dailyInput {
	t.Helper()
	in := dailyInput{Seat: s, Team: team}
	deal := medianOwned(t, e, `SELECT id::text FROM deal WHERE id = ANY($1::uuid[]) AND owner_id = $2::uuid`, c.DealIDs, c.MedianRepID)
	company := medianOwned(t, e, `SELECT id::text FROM company WHERE id = ANY($1::uuid[]) AND owner_id = $2::uuid`, c.CompanyIDs, c.MedianRepID)
	contact := medianOwned(t, e, `SELECT id::text FROM contact WHERE id = ANY($1::uuid[]) AND owner_id = $2::uuid`, c.ContactIDs, c.MedianRepID)
	project := medianOwned(t, e, `SELECT id::text FROM project WHERE id = ANY($1::uuid[]) AND owner_id = $2::uuid`, c.ProjectIDs, c.MedianRepID)
	in.DealID, in.DealName = c.DealIDs[deal], c.DealNames[deal]
	in.CompanyID, in.CompanyName = c.CompanyIDs[company], c.CompanyNames[company]
	in.ContactID, in.ProjectID = c.ContactIDs[contact], c.ProjectIDs[project]
	stem, _, _ := strings.Cut(in.CompanyName, " ")
	in.ListPrefix = string([]rune(stem)[:min(4, len([]rune(stem)))])
	return in
}

// medianOwned returns the index of the corpus row the median rep owns;
// pickCorpus guarantees one of each kind, so finding none is a broken corpus.
func medianOwned(t *testing.T, e *apptest.AppEnv, sql string, ids []string, rep string) int {
	t.Helper()
	var id string
	err := e.Owner.QueryRow(context.Background(), sql+` LIMIT 1`, ids, rep).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("the median rep owns none of the corpus rows %q picks; the flows would open a teammate's record", sql)
	}
	if err != nil {
		t.Fatalf("finding the median rep's record: %v", err)
	}
	return slices.Index(ids, id)
}

// dailyEvaluateArgs builds the evaluate query the analytics screen sends on
// open (reportingQuery in reporting.model.ts): the seat's default scope, the
// default pipeline, this month, and the sales template's metrics and blocks.
func dailyEvaluateArgs(t *testing.T, e *apptest.AppEnv, s Seat) string {
	t.Helper()
	var frame struct {
		DefaultScope struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"default_scope"`
	}
	var catalog struct {
		Metrics []struct {
			ID     string   `json:"id"`
			Blocks []string `json:"blocks"`
		} `json:"metrics"`
	}
	var pipelines struct {
		Data []struct {
			ID        string `json:"id"`
			IsDefault bool   `json:"is_default"`
		} `json:"data"`
	}
	mustCall(t, s.env(e), "GET", "/v1/analytics/context", nil, http.StatusOK, &frame)
	mustCall(t, s.env(e), "GET", "/v1/analytics/metrics", nil, http.StatusOK, &catalog)
	mustCall(t, s.env(e), "GET", "/v1/pipelines", nil, http.StatusOK, &pipelines)
	template := dailyAnalyticsTemplate(t, e, s)
	args := url.Values{}
	if frame.DefaultScope.Kind != "managed_teams" {
		args.Set("scope_kind", frame.DefaultScope.Kind)
	}
	if frame.DefaultScope.ID != "" {
		args.Set("scope_id", frame.DefaultScope.ID)
	}
	if len(pipelines.Data) > 0 && template == "sales" {
		pipeline := pipelines.Data[0].ID
		for _, p := range pipelines.Data {
			if p.IsDefault {
				pipeline = p.ID
				break
			}
		}
		args.Set("pipeline_id", pipeline)
	}
	args.Set("period", "this_month")
	args.Set("target_basis", "month")
	args.Set("close_window", "all_open")
	metrics, desired := dailyTemplateMetrics(template)
	var blocks []string
	for _, metric := range catalog.Metrics {
		if !metrics(metric.ID) {
			continue
		}
		args.Add("metrics", metric.ID)
		blocks = append(blocks, metric.Blocks...)
	}
	for _, block := range desired {
		if slices.Contains(blocks, block) {
			args.Add("blocks", block)
		}
	}
	return args.Encode()
}

// dailyAnalyticsTemplate reads the workspace's reporting template. Without a
// published framework the screen stops at an error, so the bench asks as sales.
func dailyAnalyticsTemplate(t *testing.T, e *apptest.AppEnv, s Seat) string {
	t.Helper()
	status, body, _ := s.Get(t, e, "/v1/analytics/framework")
	if status == http.StatusNotFound {
		return "sales"
	}
	var framework struct {
		Definition struct {
			Template string `json:"template"`
		} `json:"definition"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &framework) != nil || framework.Definition.Template == "" {
		t.Fatalf("GET /v1/analytics/framework answered %d: %s", status, clipBody(body))
	}
	return framework.Definition.Template
}

// dailyTemplateMetrics is the screen's metric filter and block wish list for a template.
func dailyTemplateMetrics(template string) (func(id string) bool, []string) {
	sdr := []string{"meetings_held", "accepted_opportunities"}
	if template == "sdr" {
		return func(id string) bool { return slices.Contains(sdr, id) }, []string{"sdr_outcomes", "target_progress"}
	}
	return func(id string) bool { return !slices.Contains(append(sdr, "forecast_landing"), id) },
		[]string{"bookings_trend", "stage_distribution", "owner_attainment", "stage_age"}
}
