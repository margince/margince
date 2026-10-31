// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func derefCount(count *int) int {
	if count == nil {
		return -1
	}
	return *count
}

func (f *sdrFixture) meetings(ctx context.Context, t *testing.T, owner ids.UUID, cursor *string, limit int) crmcontracts.SdrUpcomingMeetingPage {
	t.Helper()
	return f.meetingsIn(ctx, t, owner, nil, cursor, limit)
}

// meetingsIn reads one page of meetings in a window a summary declared, or in
// a window from now when window is nil.
func (f *sdrFixture) meetingsIn(ctx context.Context, t *testing.T, owner ids.UUID, window *crmcontracts.SdrUpcomingWindow, cursor *string, limit int) crmcontracts.SdrUpcomingMeetingPage {
	t.Helper()
	kind := crmcontracts.ListSdrUpcomingMeetingsParamsScopeKindOwner
	id := openapi_types.UUID(owner)
	params := crmcontracts.ListSdrUpcomingMeetingsParams{ScopeKind: &kind, ScopeId: &id, Cursor: cursor, Limit: &limit}
	if window != nil {
		params.WindowStart, params.WindowEnd = &window.StartAt, &window.EndAt
	}
	var out crmcontracts.SdrUpcomingMeetingPage
	body := sdrServe(ctx, t, f.e, func(h sdrSummaryHandlers, w http.ResponseWriter, r *http.Request) {
		h.ListSdrUpcomingMeetings(w, r, params)
	}, f.at)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f *sdrFixture) returned(ctx context.Context, t *testing.T, owner ids.UUID) crmcontracts.SdrWaitingHandoffPage {
	t.Helper()
	kind := crmcontracts.ListSdrWaitingHandoffsParamsScopeKindOwner
	state := crmcontracts.ListSdrWaitingHandoffsParamsStateReturned
	id := openapi_types.UUID(owner)
	var out crmcontracts.SdrWaitingHandoffPage
	body := sdrServe(ctx, t, f.e, func(h sdrSummaryHandlers, w http.ResponseWriter, r *http.Request) {
		h.ListSdrWaitingHandoffs(w, r, crmcontracts.ListSdrWaitingHandoffsParams{ScopeKind: &kind, ScopeId: &id, State: &state})
	}, f.at)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// logMeeting books a customer meeting through the activity writer, credited to
// the fixture's seat, starting at start with the given result.
func (f *sdrFixture) logMeeting(t *testing.T, name string, start time.Time, status string) ids.UUID {
	t.Helper()
	clock := func() time.Time { return f.at }
	customer, err := contacts.NewStore(InstallationDB(f.e.Pool)).WithClock(clock).CreateContact(f.ctx, contacts.CreateContactInput{FullName: name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	subject := "Call with " + name
	meeting, _, err := activities.NewStore(InstallationDB(f.e.Pool)).WithClock(clock).LogActivity(f.ctx, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &start, MeetingStatus: &status, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(customer.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(meeting.Id)
}

// Meetings on each side of both edges of the window. Only those that have not
// happened and start before its end count. The list's pages hold the counted
// ones and no others.
func TestSdrUpcomingMeetingsCountAndListShareOneWindow(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSales)
	e := f.e
	window := f.summary(f.ctx, t, e.Rep1).UpcomingWindow
	if window == nil || !window.StartAt.Equal(f.at) || window.Timezone == "" {
		t.Fatalf("the summary declares the window it counts in, from as_of: %+v", window)
	}
	end := window.EndAt
	if days := end.Sub(f.at); days <= 13*24*time.Hour || days > 14*24*time.Hour+time.Hour {
		t.Fatalf("the window ends at midnight fourteen days on, not %v after as_of", days)
	}
	f.logMeeting(t, "Just before now", f.at.Add(-time.Minute), "booked")
	f.logMeeting(t, "Exactly now", f.at, "booked")
	soonest := f.logMeeting(t, "Just after now", f.at.Add(time.Minute), "booked")
	latest := f.logMeeting(t, "Just before the end", end.Add(-time.Minute), "booked")
	f.logMeeting(t, "Exactly the end", end, "booked")
	f.logMeeting(t, "Just after the end", end.Add(time.Minute), "booked")
	f.logMeeting(t, "Called off", f.at.Add(48*time.Hour), "canceled")
	f.logMeeting(t, "Already held", f.at.Add(-48*time.Hour), "held")

	got := f.summary(f.ctx, t, e.Rep1)
	if got.UpcomingMeetings == nil || *got.UpcomingMeetings != 2 {
		t.Fatalf("two meetings start after now and before the end, read %d", derefCount(got.UpcomingMeetings))
	}
	first := f.meetingsIn(f.ctx, t, e.Rep1, got.UpcomingWindow, nil, 1)
	if len(first.Rows) != 1 || first.NextCursor == nil || ids.UUID(first.Rows[0].Id) != soonest {
		t.Fatalf("page one is the soonest meeting, with a cursor: %+v", first)
	}
	if !first.Window.EndAt.Equal(end) {
		t.Fatalf("the list counts in the summary's window: %v, want %v", first.Window.EndAt, end)
	}
	if row := first.Rows[0]; row.Title != "Call with Just after now" || row.Restricted || row.Record == nil || row.Record.Type != crmcontracts.SdrMeetingRecordTypeContact {
		t.Fatalf("a row names the meeting and the contact it is with: %+v", row)
	}
	second := f.meetingsIn(f.ctx, t, e.Rep1, got.UpcomingWindow, first.NextCursor, 1)
	if len(second.Rows) != 1 || second.NextCursor != nil || ids.UUID(second.Rows[0].Id) != latest {
		t.Fatalf("page two is the last counted meeting and ends the list: %+v", second)
	}
}

// A handoff sent back waits on its submitter. It leaves the waiting list and
// appears on the returned list with its reason, aged from its return.
func TestSdrReturnedHandoffsCarryTheirReason(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSdr)
	e := f.e
	handoff := f.submit(t, "Sent back prospect", &f.ae)
	var reason ids.UUID
	var label string
	if err := e.Pool.QueryRow(context.Background(), `SELECT id,label FROM sdr_handoff_reason WHERE applies_to='recycled' AND active ORDER BY sort_order LIMIT 1`).Scan(&reason, &label); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	returnedAt := f.at
	if err := f.handoffs.DecideHandoff(f.ctx, handoff, contacts.HandoffDecision{Status: contacts.HandoffRecycled, ReasonID: &reason}); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	if waiting := f.page(f.ctx, t, e.Rep1, nil, 10); len(waiting.Rows) != 0 {
		t.Fatalf("a returned handoff waits on nobody else: %+v", waiting.Rows)
	}
	got := f.returned(f.ctx, t, e.Rep1)
	if len(got.Rows) != 1 || ids.UUID(got.Rows[0].Id) != handoff {
		t.Fatalf("the returned list holds the handoff sent back: %+v", got.Rows)
	}
	row := got.Rows[0]
	if row.Reason == nil || *row.Reason != label || !row.WaitingSince.Equal(returnedAt) || row.Resubmitted {
		t.Fatalf("a returned row carries its reason and ages from the return: %+v", row)
	}
	if err := f.handoffs.ResubmitHandoff(f.ctx, handoff, nil); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	waiting := f.page(f.ctx, t, e.Rep1, nil, 10)
	if len(waiting.Rows) != 1 || !waiting.Rows[0].Resubmitted || waiting.Rows[0].Reason != nil {
		t.Fatalf("a resubmitted handoff waits again, with no reason of its own: %+v", waiting.Rows)
	}
}

// A seat reviews its own handoff rounds. Nobody else may name that seat, not a
// team manager and not a seat that may measure everyone.
func TestSdrTeamReviewReadsOneSeatsOwnRounds(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	accepted := f.submitAs(t, e.Rep1, "Accepted prospect")
	sentBack := f.submitAs(t, e.Rep1, "Sent back prospect")
	f.submitAs(t, e.Rep1, "Waiting prospect")
	f.submitAs(t, f.second, "Colleague prospect")
	deal, _, _ := reportingWrittenClose(t, e)
	dealID := ids.UUID(deal.Id)
	f.decide(t, accepted, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &dealID})
	f.decide(t, sentBack, contacts.HandoffDecision{Status: contacts.HandoffRecycled, ReasonID: f.reason(t, "recycled")})

	own := f.review(f.as(e.Rep1), t, crmcontracts.GetSdrTeamReviewParamsScopeKindOwner, &e.Rep1)
	if own.Handoffs == nil {
		t.Fatalf("a seat's own review carries its handoffs: %+v", own)
	}
	cohort := own.Handoffs.Cohort
	if cohort.Rounds != 3 || cohort.Accepted != 1 || cohort.Recycled != 1 || cohort.Waiting != 1 {
		t.Fatalf("the seat's three rounds: one accepted, one sent back, one open, read %+v", cohort)
	}
	for _, refused := range []struct {
		name   string
		reader context.Context
	}{
		{"a rep naming a colleague", sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeOwn)},
		{"a team manager naming a teammate", sectionsReader(e, f.manager, []ids.UUID{e.Team1}, principal.RowScopeTeam)},
		{"an unbounded seat naming another owner", sectionsReader(e, f.manager, []ids.UUID{e.Team1}, principal.RowScopeAll)},
	} {
		if code := f.ownReviewCode(refused.reader, f.second); code != http.StatusNotFound {
			t.Fatalf("%s gets %d for another seat's own review, want 404", refused.name, code)
		}
	}
	if code := f.ownReviewCode(sectionsReader(e, f.second, []ids.UUID{e.Team1}, principal.RowScopeOwn), f.second); code != http.StatusOK {
		t.Fatalf("a seat reading its own review gets %d", code)
	}
}

// ownReviewCode asks for owner's own review as reader and returns the status.
func (f *sdrTeamFixture) ownReviewCode(reader context.Context, owner ids.UUID) int {
	kind := crmcontracts.GetSdrTeamReviewParamsScopeKindOwner
	id := openapi_types.UUID(owner)
	w := httptest.NewRecorder()
	newSdrSummaryHandlers(InstallationDB(f.e.Pool), func() time.Time { return f.now }).GetSdrTeamReview(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(reader), crmcontracts.GetSdrTeamReviewParams{ScopeKind: &kind, ScopeId: &id})
	return w.Code
}

// A week no lead arrived in has no qualified-lead reading, while a week with
// no held meeting is a measured zero.
func TestWeeklyOutcomesLeaveAWeekWithNoCohortEmpty(t *testing.T) {
	e := setupForecast(t)
	now := clocktest.Now(t)
	arrived := now.AddDate(0, 0, -10)
	e.seedID(t, `INSERT INTO lead(id,full_name,source,captured_by,owner_id,created_at) VALUES($1,'Arrived lead','inbound','human:x',$2,$3)`, e.Rep1, arrived)
	service := newReportingService(e.Pool, func() time.Time { return now })
	admin := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeAll)
	interval := crmcontracts.ReportingWindow{StartAt: now.AddDate(0, 0, -28), EndAt: now}
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"leads_promoted", "meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{"sdr_outcomes"}}
	result, err := service.Evaluate(admin, selection)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, chart := range result.Charts {
		seen++
		measured, empty := 0, 0
		for _, point := range chart.Points {
			if point.Value == nil {
				empty++
			} else {
				measured++
			}
		}
		switch chart.Metric {
		case "leads_promoted":
			if measured != 1 || empty == 0 {
				t.Fatalf("only the week the lead arrived in has a reading, read %d measured and %d empty: %+v", measured, empty, chart.Points)
			}
		case "meetings_held":
			if empty != 0 {
				t.Fatalf("a week with no held meeting is a measured zero: %+v", chart.Points)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("both weekly charts come back, read %d", seen)
	}
}

// The clock moves on between the summary and its pages, past both meetings'
// starts. Pages read in the summary's window still hold what it counted.
func TestSdrUpcomingMeetingPagesKeepTheSummarysWindow(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSales)
	e := f.e
	first := f.logMeeting(t, "First booked", f.at.Add(time.Minute), "booked")
	second := f.logMeeting(t, "Second booked", f.at.Add(2*time.Minute), "booked")
	summary := f.summary(f.ctx, t, e.Rep1)
	if summary.UpcomingMeetings == nil || *summary.UpcomingMeetings != 2 || summary.UpcomingWindow == nil {
		t.Fatalf("the summary counts both booked meetings: %+v", summary)
	}
	window := summary.UpcomingWindow
	f.at = f.at.Add(90 * time.Second)
	one := f.meetingsIn(f.ctx, t, e.Rep1, window, nil, 1)
	if len(one.Rows) != 1 || ids.UUID(one.Rows[0].Id) != first || one.NextCursor == nil || !one.Window.StartAt.Equal(window.StartAt) {
		t.Fatalf("page one holds the first meeting the summary counted: %+v", one)
	}
	f.at = f.at.Add(time.Hour)
	two := f.meetingsIn(f.ctx, t, e.Rep1, window, one.NextCursor, 1)
	if len(two.Rows) != 1 || ids.UUID(two.Rows[0].Id) != second || two.NextCursor != nil {
		t.Fatalf("page two holds the second meeting and ends the list: %+v", two)
	}
	if now := f.meetings(f.ctx, t, e.Rep1, nil, 10); len(now.Rows) != 0 {
		t.Fatalf("a window from now holds neither, both have started: %+v", now.Rows)
	}
	kind := crmcontracts.ListSdrUpcomingMeetingsParamsScopeKindOwner
	id := openapi_types.UUID(e.Rep1)
	w := httptest.NewRecorder()
	newSdrSummaryHandlers(InstallationDB(e.Pool), func() time.Time { return f.at }).ListSdrUpcomingMeetings(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(f.ctx), crmcontracts.ListSdrUpcomingMeetingsParams{ScopeKind: &kind, ScopeId: &id, Cursor: one.NextCursor})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a cursor without its window gets %d, want 422", w.Code)
	}
}
