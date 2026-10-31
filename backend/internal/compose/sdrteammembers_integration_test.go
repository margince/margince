// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sdrLeadTeam is a qualification team whose leads come through the lead
// writers this week: Rep1 qualifies one and leaves one unanswered, a lead
// handed from Rep1 to the second SDR is qualified, a lead nobody owns is
// qualified, and a seat that joins the team afterwards brings a qualified
// lead. at is read after every write, because the lead writers stamp the
// real clock.
type sdrLeadTeam struct {
	*sdrTeamFixture
	leads  *contacts.Store
	writer context.Context
	joiner ids.UUID
	at     time.Time
}

func newSdrLeadTeam(t *testing.T) *sdrLeadTeam {
	t.Helper()
	f := &sdrLeadTeam{sdrTeamFixture: newSdrTeamFixtureFor(t, crmcontracts.ReportingFrameworkInputTemplateSales)}
	e := f.e
	f.writer = sectionsWriter(e)
	f.leads = contacts.NewStore(InstallationDB(e.Pool)).WithDealOpener(leadDealOpener{deals: deals.NewStore(InstallationDB(e.Pool), DealsInstallation())})
	f.joiner = e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'joiner@forecast.test', 'Joining SDR')`)
	f.qualify(t, f.lead(t, "Qualified by Rep1", &e.Rep1))
	f.writeIn(t, f.lead(t, "Waiting on Rep1", &e.Rep1))
	handed := f.lead(t, "Handed to the second SDR", &e.Rep1)
	second := ids.From[ids.UserKind](f.second)
	if _, err := f.leads.UpdateLead(f.writer, handed, contacts.UpdateLeadInput{OwnerID: &second}); err != nil {
		t.Fatal(err)
	}
	f.qualify(t, handed)
	f.qualify(t, f.lead(t, "Nobody's lead", nil))
	f.qualify(t, f.lead(t, "Brought by the joiner", &f.joiner))
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, f.joiner); err != nil {
		t.Fatal(err)
	}
	f.at = time.Now().UTC().Add(time.Minute)
	return f
}

func (f *sdrLeadTeam) lead(t *testing.T, name string, owner *ids.UUID) ids.LeadID {
	t.Helper()
	var ownerID *ids.UserID
	if owner != nil {
		id := ids.From[ids.UserKind](*owner)
		ownerID = &id
	}
	lead, _, err := f.leads.CreateLead(f.writer, contacts.CreateLeadInput{FullName: &name, Status: "new", OwnerID: ownerID, Source: "manual"})
	if err != nil {
		t.Fatalf("seeding lead %s: %v", name, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

func (f *sdrLeadTeam) qualify(t *testing.T, lead ids.LeadID) {
	t.Helper()
	if _, err := f.leads.QualifyLead(f.writer, lead, contacts.PromoteLeadInput{Trigger: "human_qualify", Deal: &contacts.QualifyDealInput{}}); err != nil {
		t.Fatalf("qualifying: %v", err)
	}
}

// writeIn logs the inbound email that makes a lead one that owes a reply.
func (f *sdrLeadTeam) writeIn(t *testing.T, lead ids.LeadID) {
	t.Helper()
	subject, inbound := "Can you call me back?", "inbound"
	if _, _, err := activities.NewStore(InstallationDB(f.e.Pool)).LogActivity(f.writer, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &inbound, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead.UUID}},
	}); err != nil {
		t.Fatal(err)
	}
}

// read serves this week's review of scope to ctx and answers the status.
func (f *sdrLeadTeam) read(ctx context.Context, t *testing.T, kind crmcontracts.GetSdrTeamReviewParamsScopeKind, scope *ids.UUID) (crmcontracts.SdrTeamReview, int) {
	t.Helper()
	period := crmcontracts.GetSdrTeamReviewParamsPeriodThisWeek
	clock := func() time.Time { return f.at }
	h := newSdrSummaryHandlers(InstallationDB(f.e.Pool), clock).withTargets(newReportingService(f.e.Pool, clock))
	w := httptest.NewRecorder()
	h.GetSdrTeamReview(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), crmcontracts.GetSdrTeamReviewParams{ScopeKind: &kind, ScopeId: (*openapi_types.UUID)(scope), Period: &period})
	var out crmcontracts.SdrTeamReview
	if w.Code != http.StatusOK {
		t.Logf("the team review answered %d: %s", w.Code, w.Body.String())
	}
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return out, w.Code
}

func (f *sdrLeadTeam) team(ctx context.Context, t *testing.T) crmcontracts.SdrTeamReview {
	t.Helper()
	out, code := f.read(ctx, t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &f.e.Team1)
	if code != http.StatusOK {
		t.Fatalf("the team review answered %d", code)
	}
	return out
}

// evaluate reads this week's lead readings for scope, as the screen asks.
func (f *sdrLeadTeam) evaluate(ctx context.Context, t *testing.T, scope crmcontracts.ReportingScope) crmcontracts.ReportingEvaluation {
	t.Helper()
	service := newReportingService(f.e.Pool, func() time.Time { return f.at })
	result, err := service.Evaluate(ctx, crmcontracts.ReportingSelection{
		Scope: scope, Period: "this_week", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{reportingLeadsPromoted, reportingLeadsAwaitingResponse},
		Blocks:  []crmcontracts.ReportingBlockKind{reportingSdrOutcomes},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func withoutGrants(ctx context.Context, objects ...string) context.Context {
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	for _, object := range objects {
		delete(actor.Permissions.Objects, object)
	}
	return principal.WithActor(ctx, actor)
}

func membersByID(review crmcontracts.SdrTeamReview) map[ids.UUID]crmcontracts.SdrTeamMember {
	out := map[ids.UUID]crmcontracts.SdrTeamMember{}
	for _, member := range review.Members {
		out[ids.UUID(member.Id)] = member
	}
	return out
}

func memberFigure(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}

// Each member's qualified and unanswered leads are the governed readings'
// values for that member alone, credited to each lead's owner now.
func TestSdrTeamMembersCountLeadsByTheReadingsOwnRules(t *testing.T) {
	f := newSdrLeadTeam(t)
	e := f.e
	manager := f.as(f.manager)
	got := f.team(manager, t)
	if got.Workflow != crmcontracts.SdrTeamReviewWorkflowQualification || len(got.WithheldFigures) != 0 {
		t.Fatalf("a qualification team with every grant: %+v", got)
	}
	rows := membersByID(got)
	want := map[ids.UUID][2]int{e.Rep1: {1, 1}, f.second: {1, 0}, f.joiner: {1, 0}}
	for seat, figures := range want {
		row := rows[seat]
		if memberFigure(row.Qualified) != figures[0] || memberFigure(row.Unanswered) != figures[1] {
			t.Errorf("%s: qualified %d, unanswered %d, want %v", row.Label, memberFigure(row.Qualified), memberFigure(row.Unanswered), figures)
		}
		reading := f.evaluate(manager, t, crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(seat)})
		if sectionsValue(t, reading, reportingLeadsPromoted) != float64(memberFigure(row.Qualified)) ||
			sectionsValue(t, reading, reportingLeadsAwaitingResponse) != float64(memberFigure(row.Unanswered)) {
			t.Errorf("%s's row differs from the readings for that seat: %+v", row.Label, reading.Metrics)
		}
	}
	team := f.evaluate(manager, t, crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)})
	if sectionsValue(t, team, reportingLeadsPromoted) != 3 {
		t.Fatalf("the team's reading counts its three owned qualified leads and not the unowned one: %+v", team.Metrics)
	}
}

// A target is held against the month an evaluation with a monthly basis reads,
// and only for the member who has one.
func TestSdrTeamMembersHoldOnlyTheirOwnMonthlyTarget(t *testing.T) {
	f := newSdrLeadTeam(t)
	e := f.e
	manager := f.as(f.manager)
	first := f.team(manager, t)
	if first.MemberTargets == nil || first.MemberTargets.Metric != crmcontracts.SdrTeamMemberTargetsMetricLeadsPromoted {
		t.Fatalf("a reader of targets is told the month targets stand against: %+v", first.MemberTargets)
	}
	window := first.MemberTargets.Window
	if !window.EndAt.Equal(window.StartAt.AddDate(0, 1, 0)) || window.EndAt.Before(f.at) {
		t.Fatalf("the target window is the whole month that holds the cutoff: %+v", window)
	}
	if _, err := newReportingService(e.Pool, func() time.Time { return f.at }).CreateTarget(f.ctx, crmcontracts.ReportingTargetInput{
		Metric: reportingLeadsPromoted, Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)},
		PeriodKind: "month", PeriodStart: openapi_types.Date{Time: window.StartAt}, Value: 4, Reason: "Month plan",
	}); err != nil {
		t.Fatal(err)
	}
	rows := membersByID(f.team(manager, t))
	if target := rows[e.Rep1].Target; target == nil || target.Value != 4 || target.Actual != 1 {
		t.Fatalf("Rep1 stands against their target of 4 with one qualified lead: %+v", target)
	}
	if rows[f.second].Target != nil {
		t.Fatalf("a member with no target has none: %+v", rows[f.second].Target)
	}
	// The reporting service answers to the target grant itself, whoever asks it.
	err := InstallationDB(e.Pool).Tx(manager, func(tx pgx.Tx) error {
		targets, readable, err := newReportingService(e.Pool, func() time.Time { return f.at }).OwnerTargets(
			withoutGrants(manager, "sales_target"), tx, crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, reportingLeadsPromoted, window, []ids.UUID{e.Rep1})
		if err == nil && (readable || len(targets) != 0) {
			t.Errorf("a reader without target read learns targets: %v %v", readable, targets)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Each figure answers to the reader's own grant: activity read for meetings,
// lead read for leads, target read for targets. A grant taken away shows on
// the next read.
func TestSdrTeamMembersWithholdEachFigureByTheReadersGrant(t *testing.T) {
	f := newSdrLeadTeam(t)
	manager := f.as(f.manager)
	for _, tc := range []struct {
		name     string
		reader   context.Context
		withheld []crmcontracts.SdrTeamReviewWithheldFigures
		leads    bool
	}{
		{"every grant", manager, nil, true},
		{"no activity read", withoutGrants(manager, "activity"), []crmcontracts.SdrTeamReviewWithheldFigures{crmcontracts.SdrTeamReviewWithheldFiguresMeetings}, true},
		{"no target read", withoutGrants(manager, "sales_target"), []crmcontracts.SdrTeamReviewWithheldFigures{crmcontracts.SdrTeamReviewWithheldFiguresTargets}, true},
		{"lead read revoked", withoutGrants(manager, "lead"), []crmcontracts.SdrTeamReviewWithheldFigures{crmcontracts.SdrTeamReviewWithheldFiguresHandoffs, crmcontracts.SdrTeamReviewWithheldFiguresLeads, crmcontracts.SdrTeamReviewWithheldFiguresTargets}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := f.team(tc.reader, t)
			if !slices.Equal(got.WithheldFigures, append([]crmcontracts.SdrTeamReviewWithheldFigures{}, tc.withheld...)) {
				t.Fatalf("withheld figures = %v, want %v", got.WithheldFigures, tc.withheld)
			}
			meetings := !slices.Contains(tc.withheld, crmcontracts.SdrTeamReviewWithheldFiguresMeetings)
			targets := !slices.Contains(tc.withheld, crmcontracts.SdrTeamReviewWithheldFiguresTargets)
			if (got.MemberTargets != nil) != targets {
				t.Fatalf("the target month is named only to a reader of targets: %+v", got.MemberTargets)
			}
			for _, row := range got.Members {
				if (row.Qualified != nil) != tc.leads || (row.Unanswered != nil) != tc.leads || (row.MeetingsHeld != nil) != meetings {
					t.Fatalf("%s: qualified %d, unanswered %d, meetings %v", row.Label, memberFigure(row.Qualified), memberFigure(row.Unanswered), row.MeetingsHeld)
				}
				// Handoffs are kept with the leads, so even a qualification team's
				// handoff counts are absent, not zero, without lead read.
				for _, handoff := range []*int{row.Submitted, row.Accepted, row.Rejected, row.Recycled, row.Waiting} {
					if (handoff != nil) != tc.leads {
						t.Fatalf("%s's handoff counts reach a reader without the grant: %+v", row.Label, row)
					}
				}
			}
		})
	}
}

// Rep1 under their own row scope reads their own row and no colleague's, and
// a team lead reads the team they lead.
func TestSdrTeamMembersFollowTheReadersReach(t *testing.T) {
	f := newSdrLeadTeam(t)
	e := f.e
	own := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeOwn)
	actor, _ := principal.Actor(own)
	actor.Permissions.Objects[contacts.HandoffObject] = principal.ObjectGrant{Read: true}
	own = principal.WithActor(own, actor)
	mine, code := f.read(own, t, crmcontracts.GetSdrTeamReviewParamsScopeKindOwner, &e.Rep1)
	if code != http.StatusOK || len(mine.Members) != 1 || ids.UUID(mine.Members[0].Id) != e.Rep1 || memberFigure(mine.Members[0].Qualified) != 1 {
		t.Fatalf("Rep1 reads their own row: %d %+v", code, mine.Members)
	}
	if _, code := f.read(own, t, crmcontracts.GetSdrTeamReviewParamsScopeKindOwner, &f.second); code != http.StatusNotFound {
		t.Fatalf("a colleague's own review is not Rep1's to read: %d", code)
	}
	lead := sectionsReader(e, f.manager, []ids.UUID{e.Team1}, principal.RowScopeTeam)
	actor, _ = principal.Actor(lead)
	actor.Permissions.Objects[contacts.HandoffObject] = principal.ObjectGrant{Read: true}
	lead = principal.WithActor(lead, actor)
	team := f.team(lead, t)
	if rows := membersByID(team); len(rows) != 3 || memberFigure(rows[f.joiner].Qualified) != 1 {
		t.Fatalf("a team lead reads every member of today's team: %+v", team.Members)
	}
	for _, row := range team.Members {
		if ids.UUID(row.Id) == e.Rep3 {
			t.Fatalf("a seat of another team is in Team1's rows: %+v", row)
		}
	}
}

// With individual performance off the review names no member, and the
// evidence and export of the team's readings name no colleague either.
func TestSdrTeamMembersNameNobodyWhileIndividualPerformanceIsOff(t *testing.T) {
	f := newSdrLeadTeam(t)
	e := f.e
	e.showIndividualPerformance(t, false)
	manager := f.as(f.manager)
	got := f.team(manager, t)
	if !got.MembersWithheld || len(got.Members) != 0 || got.MemberTargets != nil {
		t.Fatalf("no member row and no target month while the switch is off: %+v", got)
	}
	team := f.evaluate(manager, t, crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)})
	for _, row := range sectionsEvidence(manager, t, newReportingService(e.Pool, func() time.Time { return f.at }), team, reportingLeadsPromoted, "interval", "") {
		if row.OwnerId != nil {
			t.Fatalf("the drill-down names a colleague as a lead's owner: %+v", row)
		}
	}
	csv, err := reportingCSV(team)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Second SDR", "Joining SDR"} {
		if strings.Contains(string(csv), name) {
			t.Fatalf("the export names %s:\n%s", name, csv)
		}
	}
}

// A team's list of waiting handoffs says who submitted each only while the
// installation shows individual performance, and always to the submitter.
func TestSdrWaitingHandoffsNameNoColleagueSubmitterWhileIndividualPerformanceIsOff(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	f.submitAs(t, e.Rep1, "Rep1's prospect")
	f.at = f.at.Add(time.Hour)
	f.submitAs(t, f.second, "Second SDR's prospect")
	e.showIndividualPerformance(t, false)
	list := func(ctx context.Context) crmcontracts.SdrWaitingHandoffPage {
		kind := crmcontracts.ListSdrWaitingHandoffsParamsScopeKindTeam
		limit := 50
		var out crmcontracts.SdrWaitingHandoffPage
		body := sdrServe(ctx, t, e, func(h sdrSummaryHandlers, w http.ResponseWriter, r *http.Request) {
			h.ListSdrWaitingHandoffs(w, r, crmcontracts.ListSdrWaitingHandoffsParams{ScopeKind: &kind, ScopeId: (*openapi_types.UUID)(&e.Team1), Limit: &limit})
		}, f.now)
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	page := list(f.as(f.manager))
	if len(page.Rows) != 2 {
		t.Fatalf("both prospects still wait: %+v", page.Rows)
	}
	for _, row := range page.Rows {
		if row.SubmitterId != nil {
			t.Fatalf("the manager's list names a submitter: %+v", row)
		}
	}
	own := 0
	for _, row := range list(f.as(e.Rep1)).Rows {
		if row.SubmitterId != nil && ids.UUID(*row.SubmitterId) == e.Rep1 {
			own++
		} else if row.SubmitterId != nil {
			t.Fatalf("Rep1's list names a colleague: %+v", row)
		}
	}
	if own != 1 {
		t.Fatalf("Rep1 still sees their own submission: %d", own)
	}
}

// The reasons for one outcome add up to that outcome's count in the period, a
// decision with no reason included, so the drill-down holds the same
// population as the figure.
func TestSdrTeamReasonsAddUpToTheirOutcome(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	for _, name := range []string{"First returned", "Second returned"} {
		handoff := f.submitAs(t, e.Rep1, name)
		f.decide(t, handoff, contacts.HandoffDecision{Status: contacts.HandoffRecycled, ReasonID: f.reason(t, contacts.HandoffRecycled)})
	}
	rejected := f.submitAs(t, f.second, "Rejected")
	f.decide(t, rejected, contacts.HandoffDecision{Status: contacts.HandoffRejected, ReasonID: f.reason(t, contacts.HandoffRejected)})
	legacy := f.submitAs(t, f.second, "Rejected before reasons were kept")
	f.decide(t, legacy, contacts.HandoffDecision{Status: contacts.HandoffRejected, ReasonID: f.reason(t, contacts.HandoffRejected)})
	if _, err := e.owner.Exec(context.Background(), `UPDATE sdr_handoff_event SET reason_id=NULL,reason_applies_to=NULL WHERE handoff_id=$1 AND to_status='rejected'`, legacy); err != nil {
		t.Fatal(err)
	}
	got := f.review(f.as(f.manager), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	if got.Handoffs == nil {
		t.Fatalf("an SDR team has handoffs: %+v", got)
	}
	sums := map[crmcontracts.SdrHandoffReasonCountOutcome]int{}
	unspecified := 0
	for _, reason := range got.Handoffs.Reasons {
		sums[reason.Outcome] += reason.Count
		if !reason.Given {
			unspecified += reason.Count
			if reason.Label != "" {
				t.Fatalf("a decision with no reason has no label: %+v", reason)
			}
		}
	}
	outcomes := got.Handoffs.Outcomes
	if sums[crmcontracts.SdrHandoffReasonCountOutcomeRecycled] != outcomes.Recycled || sums[crmcontracts.SdrHandoffReasonCountOutcomeRejected] != outcomes.Rejected || unspecified != 1 {
		t.Fatalf("reasons %v do not add up to outcomes %+v, with one unspecified (%d)", sums, outcomes, unspecified)
	}
}

// A reader who may read activities and not leads still learns the meetings
// held, and nothing of the handoffs, which are kept with the leads.
func TestSdrTeamReviewKeepsMeetingsForAReaderWithoutLeadRead(t *testing.T) {
	f := newSdrTeamFixture(t)
	e := f.e
	f.submitAs(t, e.Rep1, "Handed on")
	f.at = f.at.Add(time.Hour)
	f.holdMeeting(t)
	got := f.review(withoutGrants(f.as(f.manager), "lead"), t, crmcontracts.GetSdrTeamReviewParamsScopeKindTeam, &e.Team1)
	want := []crmcontracts.SdrTeamReviewWithheldFigures{crmcontracts.SdrTeamReviewWithheldFiguresHandoffs, crmcontracts.SdrTeamReviewWithheldFiguresLeads}
	if got.Workflow != crmcontracts.SdrTeamReviewWorkflowHandoff || got.Handoffs != nil || got.Meetings == nil || !slices.Equal(got.WithheldFigures, want) {
		t.Fatalf("meetings stay and handoffs go: %+v", got)
	}
	for _, row := range membersByID(got) {
		if row.MeetingsHeld == nil || *row.MeetingsHeld != 1 || row.Submitted != nil || row.Waiting != nil || row.Qualified != nil {
			t.Fatalf("%s keeps the meeting and loses the handoff and lead figures: %+v", row.Label, row)
		}
	}
}
