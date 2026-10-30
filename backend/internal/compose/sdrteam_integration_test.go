// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/reportperiod"
)

// sdrTeamFixture is Team1 with two SDRs, Rep1 and second, whose work happened
// last week. now is when the review is read.
type sdrTeamFixture struct {
	*sdrFixture
	second, manager ids.UUID
	now             time.Time
}

func newSdrTeamFixture(t *testing.T) *sdrTeamFixture {
	t.Helper()
	f := &sdrTeamFixture{sdrFixture: newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSdr), now: clocktest.Now(t).UTC()}
	f.at = reportperiod.WeekStart(f.now, time.UTC).AddDate(0, 0, -6).Add(9 * time.Hour)
	f.second = f.e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'second@forecast.test', 'Second SDR')`)
	f.manager = f.e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'manager@forecast.test', 'Manager')`)
	if _, err := f.e.owner.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, f.e.Team1, f.second); err != nil {
		t.Fatal(err)
	}
	return f
}

// as is the fixture's seat acting as user, with the same grants.
func (f *sdrTeamFixture) as(user ids.UUID) context.Context {
	actor, _ := principal.Actor(f.ctx)
	actor.UserID, actor.ID = user, "human:"+user.String()
	return principal.WithActor(f.ctx, actor)
}

func (f *sdrTeamFixture) submitAs(t *testing.T, user ids.UUID, name string) ids.UUID {
	t.Helper()
	return f.submitTo(t, user, name, &f.ae)
}

// submitTo hands a prospect on to recipient, or to the shared queue for nil.
func (f *sdrTeamFixture) submitTo(t *testing.T, user ids.UUID, name string, recipient *ids.UUID) ids.UUID {
	t.Helper()
	contact := f.e.seedID(t, `INSERT INTO contact(id,full_name,source,captured_by) VALUES($1,$2,'manual','system:fixture')`, name)
	id, err := f.handoffs.SubmitHandoff(f.as(user), contacts.NewSDRHandoff{ContactID: &contact, AssignedTo: recipient})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *sdrTeamFixture) decide(t *testing.T, handoff ids.UUID, decision contacts.HandoffDecision) {
	t.Helper()
	f.at = f.at.Add(time.Hour)
	if err := f.handoffs.DecideHandoff(f.ctx, handoff, decision); err != nil {
		t.Fatal(err)
	}
}

func (f *sdrTeamFixture) reason(t *testing.T, appliesTo string) *ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := f.e.Pool.QueryRow(context.Background(), `SELECT id FROM sdr_handoff_reason WHERE applies_to=$1 AND active ORDER BY sort_order LIMIT 1`, appliesTo).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return &id
}

func (f *sdrTeamFixture) review(ctx context.Context, t *testing.T, kind crmcontracts.GetSdrTeamReviewParamsScopeKind, scope *ids.UUID) crmcontracts.SdrTeamReview {
	t.Helper()
	period := crmcontracts.GetSdrTeamReviewParamsPeriodLastWeek
	var out crmcontracts.SdrTeamReview
	body := sdrServe(ctx, t, f.e, func(h sdrSummaryHandlers, w http.ResponseWriter, r *http.Request) {
		h.GetSdrTeamReview(w, r, crmcontracts.GetSdrTeamReviewParams{ScopeKind: &kind, ScopeId: (*openapi_types.UUID)(scope), Period: &period})
	}, f.now)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// holdMeeting logs a held customer meeting as Rep1 that the second SDR
// attended too, so both are credited with it.
func (f *sdrTeamFixture) holdMeeting(t *testing.T) {
	t.Helper()
	clock := func() time.Time { return f.at }
	customer, err := contacts.NewStore(InstallationDB(f.e.Pool)).WithClock(clock).CreateContact(f.ctx, contacts.CreateContactInput{FullName: "Customer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	subject, start, held := "Discovery", f.at.Add(-time.Hour), "held"
	meeting, _, err := activities.NewStore(InstallationDB(f.e.Pool)).WithClock(clock).LogActivity(f.ctx, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &start, MeetingStatus: &held, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(customer.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.owner.Exec(context.Background(), `INSERT INTO activity_participant (activity_id, role, address, user_id, response) VALUES ($1, 'attendee', 'second@forecast.test', $2, 'accepted')`, meeting.Id, f.second); err != nil {
		t.Fatal(err)
	}
}

// One prospect is sent back, resubmitted and accepted; another is rejected;
// the second SDR's waits. Both SDRs held one meeting together.
func TestSdrTeamReviewSeparatesOutcomesFromRoundsAndCountsSharedWorkOnce(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	returned := f.submitAs(t, e.Rep1, "Returned then accepted")
	f.decide(t, returned, contacts.HandoffDecision{Status: contacts.HandoffRecycled, ReasonID: f.reason(t, contacts.HandoffRecycled)})
	f.at = f.at.Add(time.Hour)
	if err := f.handoffs.ResubmitHandoff(f.ctx, returned, nil); err != nil {
		t.Fatal(err)
	}
	deal, _, _ := reportingWrittenClose(t, e)
	dealID := ids.UUID(deal.Id)
	f.decide(t, returned, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &dealID})
	rejected := f.submitAs(t, e.Rep1, "Rejected")
	f.decide(t, rejected, contacts.HandoffDecision{Status: contacts.HandoffRejected, ReasonID: f.reason(t, contacts.HandoffRejected)})
	f.at = f.at.Add(time.Hour)
	f.submitAs(t, f.second, "Still waiting")
	f.at = f.at.Add(time.Hour)
	f.holdMeeting(t)

	got := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	if got.Workflow != crmcontracts.SdrTeamReviewWorkflowHandoff || got.Handoffs == nil || got.Meetings == nil || got.Withheld {
		t.Fatalf("an SDR framework reviews handoffs and meetings: %+v", got)
	}
	handoffs := *got.Handoffs
	if want := (crmcontracts.SdrHandoffOutcomes{Submitted: 4, Accepted: 1, Rejected: 1, Recycled: 1}); handoffs.Outcomes != want {
		t.Errorf("outcomes = %+v, want %+v: a return and a rejection are two events, and the resubmitted prospect is one acceptance", handoffs.Outcomes, want)
	}
	cohort := handoffs.Cohort
	if cohort.Rounds != 4 || cohort.Accepted != 1 || cohort.Rejected != 1 || cohort.Recycled != 1 || cohort.Waiting != 1 || cohort.MedianDecisionHours == nil {
		t.Errorf("cohort = %+v, want four rounds that each ended once, one still waiting", cohort)
	}
	if handoffs.Waiting.Waiting != 1 || len(handoffs.Reasons) != 2 || handoffs.Reasons[0].Outcome == handoffs.Reasons[1].Outcome {
		t.Errorf("one handoff waits now and the two reasons stay apart: waiting %+v, reasons %+v", handoffs.Waiting, handoffs.Reasons)
	}
	if got.Meetings.Held != 1 || got.SmallSampleBelow != sdrTeamSmallSample {
		t.Errorf("the shared meeting counts once for the team: %+v", got.Meetings)
	}
	rows := map[ids.UUID]crmcontracts.SdrTeamMember{}
	for _, row := range got.Members {
		rows[ids.UUID(row.Id)] = row
	}
	first, second := rows[e.Rep1], rows[f.second]
	if len(got.Members) != 2 || got.MembersWithheld || got.MembersTruncated {
		t.Fatalf("members = %+v, want one row per SDR", got.Members)
	}
	if first.MeetingsHeld != 1 || first.Submitted != 3 || first.Accepted != 1 || first.Rejected != 1 || first.Recycled != 1 || first.Waiting != 0 {
		t.Errorf("Rep1's row = %+v", first)
	}
	if second.MeetingsHeld != 1 || second.Submitted != 1 || second.Accepted != 0 || second.Waiting != 1 || second.OldestWaitingSince == nil {
		t.Errorf("the second SDR's row = %+v; a meeting both attended is in each row", second)
	}
}

func TestSdrTeamReviewWithIndividualPerformanceOffKeepsTeamTotalsOnly(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	f.submitAs(t, e.Rep1, "First SDR's")
	f.at = f.at.Add(time.Hour)
	f.submitAs(t, f.second, "Second SDR's")
	// The manager is on the team too, so their own row would be the one a
	// leak shows.
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, f.manager); err != nil {
		t.Fatal(err)
	}
	e.showIndividualPerformance(t, false)

	got := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	if got.Withheld || got.Handoffs == nil || got.Handoffs.Outcomes.Submitted != 2 {
		t.Fatalf("two colleagues' work is a team total the manager may read: %+v", got)
	}
	if !got.MembersWithheld || len(got.Members) != 0 {
		t.Fatalf("no row names a member while individual performance is off: %+v", got.Members)
	}
	// To Rep1 the same total minus their own is the second SDR's figure.
	mine := f.review(f.as(e.Rep1), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	if !mine.Withheld || mine.Handoffs != nil || mine.Meetings != nil || len(mine.Members) != 0 {
		t.Fatalf("a total one colleague makes up is withheld: %+v", mine)
	}
}

func TestSdrTeamReviewCountsAnUnattributedEventForNoMember(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	legacy := f.submitAs(t, e.Rep1, "Recorded before attribution")
	if _, err := e.owner.Exec(context.Background(), `UPDATE sdr_handoff_event SET submitter_id_at_change=NULL WHERE handoff_id=$1`, legacy); err != nil {
		t.Fatal(err)
	}
	team := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	if team.Handoffs == nil || team.Handoffs.Outcomes.Submitted != 0 || team.Handoffs.Unattributed != 0 {
		t.Fatalf("a team neither claims an event that names no submitter nor learns how many the company holds: %+v", team.Handoffs)
	}
	for _, row := range team.Members {
		if row.Submitted != 0 || row.Waiting != 0 {
			t.Fatalf("an unattributed event is no member's history: %+v", row)
		}
	}
	company := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindWorkspace, nil)
	if company.Handoffs == nil || company.Handoffs.Outcomes.Submitted != 1 || company.Handoffs.Unattributed != 1 {
		t.Fatalf("the whole company counts it: %+v", company.Handoffs)
	}
}

// Two SDRs hand on the same company and both handoffs are accepted onto one
// deal. The deal is one accepted opportunity, credited to the first.
func TestSdrTeamReviewCreditsADealAcceptedTwiceOnce(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	first := f.submitAs(t, e.Rep1, "First to hand it on")
	f.at = f.at.Add(time.Hour)
	second := f.submitAs(t, f.second, "Second to hand it on")
	deal, _, _ := reportingWrittenClose(t, e)
	dealID := ids.UUID(deal.Id)
	f.decide(t, first, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &dealID})
	f.decide(t, second, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &dealID})

	got := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	if got.Handoffs == nil || got.Handoffs.Outcomes.Accepted != 1 || got.Handoffs.Cohort.Accepted != 2 {
		t.Fatalf("one deal is one accepted opportunity, though both rounds ended accepted: %+v", got.Handoffs)
	}
	for _, row := range got.Members {
		want := 0
		if ids.UUID(row.Id) == e.Rep1 {
			want = 1
		}
		if row.Accepted != want {
			t.Errorf("%s is credited %d accepted, want %d: the deal goes to the first acceptance", row.Label, row.Accepted, want)
		}
	}
}

// Three SDRs each hand one prospect on last week. Two are accepted then, and
// the third is decided only this week, so last week's rounds still waiting
// are that one colleague's alone.
func TestSdrTeamReviewJudgesEachRoundFigureByItsOwnSeats(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	third := e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'third@forecast.test', 'Third SDR')`)
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, third); err != nil {
		t.Fatal(err)
	}
	won, store, writer := reportingWrittenClose(t, e)
	owner := ids.From[ids.UserKind](e.Rep1)
	other, err := store.CreateDeal(writer, deals.CreateDealInput{Name: "A second deal", PipelineID: ids.From[ids.PipelineKind](e.pipeline), StageID: ids.From[ids.StageKind](e.stages[20]), OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for index, seat := range []ids.UUID{e.Rep1, f.second} {
		handoff := f.submitAs(t, seat, "Accepted "+seat.String())
		dealID := []ids.UUID{ids.UUID(won.Id), ids.UUID(other.Id)}[index]
		f.decide(t, handoff, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &dealID})
	}
	late := f.submitAs(t, third, "Decided this week")
	f.at = f.now.Add(-2 * time.Hour)
	f.decide(t, late, contacts.HandoffDecision{Status: contacts.HandoffRejected, ReasonID: f.reason(t, contacts.HandoffRejected)})
	e.showIndividualPerformance(t, false)

	got := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	if !got.Withheld || got.Handoffs != nil {
		t.Fatalf("the rounds still waiting are one colleague's, so the review is withheld: %+v", got.Handoffs)
	}
}

// Handoffs from before the period: one SDR's is waiting on a decision, the
// other's was sent back to them. Each count is one colleague's.
func TestSdrTeamReviewJudgesWaitingAndReturnedApart(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	f.at = f.at.AddDate(0, 0, -14)
	f.submitAs(t, e.Rep1, "Waiting from before")
	returned := f.submitAs(t, f.second, "Returned from before")
	f.decide(t, returned, contacts.HandoffDecision{Status: contacts.HandoffRecycled, ReasonID: f.reason(t, contacts.HandoffRecycled)})
	e.showIndividualPerformance(t, false)

	got := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	if !got.Withheld || got.Handoffs != nil {
		t.Fatalf("waiting and returned are each one colleague's count, so the review is withheld: %+v", got.Handoffs)
	}
}

// Two SDRs' handoffs from before the period still wait, so the waiting count
// has two seats behind it. Only one SDR's is in the shared queue, and only
// one SDR's recipient can no longer act.
func TestSdrTeamReviewJudgesUnassignedAndBlockedApart(t *testing.T) {
	for _, tc := range []struct {
		name   string
		second func(f *sdrTeamFixture) *ids.UUID
	}{
		{"unassigned", func(*sdrTeamFixture) *ids.UUID { return nil }},
		{"blocked", func(f *sdrTeamFixture) *ids.UUID { return &f.e.Rep3 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSdrTeamFixture(t)
			e := f.e
			f.at = f.at.AddDate(0, 0, -14)
			f.submitAs(t, e.Rep1, "Waiting on the AE")
			f.submitTo(t, f.second, "Waiting elsewhere", tc.second(f))
			if _, err := e.Pool.Exec(context.Background(), `UPDATE app_user SET status='suspended' WHERE id=$1`, e.Rep3); err != nil {
				t.Fatal(err)
			}
			e.showIndividualPerformance(t, false)
			got := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
			if !got.Withheld || got.Handoffs != nil {
				t.Fatalf("the %s count is one colleague's, so the review is withheld: %+v", tc.name, got.Handoffs)
			}
		})
	}
}
