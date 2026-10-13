// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A team's figures sum its members' weeks, so one member's earlier record
// measures the team's week even where another member holds none.
func TestATeamFigureIsMeasuredFromAnyMembersFirstRecord(t *testing.T) {
	e := integration.Setup(t)
	held := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	logHeldMeeting(t, e, e.Rep2, held)
	engine := newWeeklyEngine(e.Pool)
	for _, rep := range []ids.UUID{e.Rep1, e.Rep2} {
		assembleMemberWeek(t, e, engine, rep)
	}

	team := assembleTeamCoverage(t, e, engine)
	if team.Meetings == nil || team.Meetings.Status != crmcontracts.WeeklyFigureCoverageStatusRecorded ||
		team.Meetings.RecordedSince == nil || !team.Meetings.RecordedSince.Equal(held) {
		t.Fatalf("team meetings = %+v, want recorded from the member's meeting on %s", team.Meetings, held)
	}
	if team.Deals == nil || team.Deals.Status != crmcontracts.WeeklyFigureCoverageStatusNotRecorded || team.Deals.RecordedSince != nil {
		t.Errorf("team deals for members who hold none = %+v, want not_recorded", team.Deals)
	}
	if team.Commitments == nil {
		t.Error("every member week settled its plan, yet the team states no commitments coverage")
	}
}

// One member week that settled no plan leaves the summed commitments figure
// unmeasured, so the team states nothing about it.
func TestATeamStatesNoCommitmentsWhenAMemberWeekSettledNoPlan(t *testing.T) {
	e := integration.Setup(t)
	planned := newWeeklyEngine(e.Pool)
	unplanned := weekly.NewEngine(e.Pool, newTeammatesSeam(e.Pool)).WithNumeric(weeklyNumericEvaluator{})
	own := assembleMemberWeek(t, e, planned, e.Rep1)
	if own.Commitments == nil {
		t.Fatal("a member week that settled its plan states no commitments coverage")
	}
	if silent := assembleMemberWeek(t, e, unplanned, e.Rep2); silent.Commitments != nil {
		t.Fatalf("a member week with no plan engine states commitments: %+v", silent.Commitments)
	}

	team := assembleTeamCoverage(t, e, planned)
	if team.Commitments != nil {
		t.Errorf("the team states commitments %+v over a member week that settled no plan", team.Commitments)
	}
	if team.Deals == nil || team.Tasks == nil || team.Meetings == nil || team.Leads == nil {
		t.Errorf("the record families lost their statements beside it: %+v", team)
	}
}

func logHeldMeeting(t *testing.T, e *integration.Env, host ids.UUID, at time.Time) {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Anna Weber"})
	if err != nil {
		t.Fatal(err)
	}
	status, subject := "held", "Quarterly review"
	if _, _, err := e.Activities.LogActivity(e.As(host, []ids.UUID{e.Team1}, integration.AdminPerms), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, MeetingStatus: &status, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(contact.Id)}},
	}); err != nil {
		t.Fatal(err)
	}
}

func assembleMemberWeek(t *testing.T, e *integration.Env, engine *weekly.Engine, rep ids.UUID) *crmcontracts.WeeklyFigureCoverageSet {
	t.Helper()
	review, _, err := engine.AssembleFor(e.As(rep, []ids.UUID{e.Team1}, integration.AdminPerms), teamJobClock)
	if err != nil {
		t.Fatalf("writing %v's week: %v", rep, err)
	}
	if review.NumericSummary == nil || review.NumericSummary.FigureCoverage == nil {
		t.Fatalf("%v's week carries no figure coverage", rep)
	}
	return review.NumericSummary.FigureCoverage
}

func assembleTeamCoverage(t *testing.T, e *integration.Env, engine *weekly.Engine) *crmcontracts.WeeklyFigureCoverageSet {
	t.Helper()
	members := []weekly.TeamMember{{UserID: e.Rep1, DisplayName: "Rep 1"}, {UserID: e.Rep2, DisplayName: "Rep 2"}}
	review, _, err := engine.AssembleTeamFor(
		e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), e.Team1, "Team 1", members, teamJobClock)
	if err != nil {
		t.Fatalf("writing the team's week: %v", err)
	}
	if review.RepsUnread != 0 || review.NumericSummary == nil || review.NumericSummary.FigureCoverage == nil {
		t.Fatalf("the team week counted no members or carries no coverage: %+v", review)
	}
	return review.NumericSummary.FigureCoverage
}
