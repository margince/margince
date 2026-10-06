// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a lead's team read says about the team's weekly plans: each teammate's
// due promise under their own name, and which plans were never looked at.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// plansByOwner answers each owner's due plan, or that owner's failure, and
// records whose plans were asked for.
type plansByOwner struct {
	due    map[ids.UUID][]PlanWork
	failed map[ids.UUID]error
	asked  *[]ids.UUID
}

func (p plansByOwner) DuePlan(_ context.Context, owner ids.UUID, _ time.Time) ([]PlanWork, error) {
	if p.asked != nil {
		*p.asked = append(*p.asked, owner)
	}
	if err := p.failed[owner]; err != nil {
		return nil, err
	}
	return p.due[owner], nil
}

func promiseBy(owner ids.UUID, label string) PlanWork {
	return PlanWork{ID: ids.NewV7(), OwnerID: owner, Label: label, DueAt: rankInstant.Add(-time.Hour)}
}

var theTeam = roster{
	{UserID: theReader, DisplayName: "Aa Reader"},
	{UserID: theColleague, DisplayName: "Bb Colleague"},
}

func teamPlanService(teammates Teammates, plans WeeklyPlans) *Service {
	return meetingPrepService(nil).WithTeammates(teammates).WithWeeklyPlans(plans)
}

func planRowOwners(day crmcontracts.Worklist) map[string]ids.UUID {
	owners := map[string]ids.UUID{}
	for _, row := range day.Queue {
		if row.Source == sourceWeeklyCommitment && row.Owner != nil && row.Owner.Id != nil {
			owners[*row.Title] = ids.UUID(*row.Owner.Id)
		}
	}
	return owners
}

func planSourceMissing(day crmcontracts.Worklist) (crmcontracts.WorklistSourceUnavailableReason, bool) {
	for _, missing := range day.SourcesUnavailable {
		if missing.Source == sourceWeeklyCommitment {
			return missing.Reason, true
		}
	}
	return "", false
}

func TestATeamReadCarriesEachTeammatesDuePromiseUnderTheirName(t *testing.T) {
	t.Parallel()
	svc := teamPlanService(theTeam, plansByOwner{due: map[ids.UUID][]PlanWork{
		theReader:    {promiseBy(theReader, "Send the proposal")},
		theColleague: {promiseBy(theColleague, "Call the buyer back")},
	}})
	day, err := svc.Worklist(meetingPrepReader(), scopeTeam, "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	owners := planRowOwners(day)
	if owners["Send the proposal"] != theReader || owners["Call the buyer back"] != theColleague {
		t.Fatalf("each promise must sit under the teammate who made it: %v", owners)
	}
	if reason, missing := planSourceMissing(day); missing {
		t.Fatalf("a team whose plans all answered reported the source %s", reason)
	}
	if day.PlanCoverage == nil || len(day.PlanCoverage.Members) != 2 || day.PlanCoverage.Truncated {
		t.Fatalf("coverage must name both teammates, untruncated: %+v", day.PlanCoverage)
	}
	for _, member := range day.PlanCoverage.Members {
		if !member.Read {
			t.Errorf("%s answered but is reported unread", member.DisplayName)
		}
	}
	if len(svc.planRows) != 0 || svc.planCoverage != nil {
		t.Fatal("one reader's team plans escaped into the shared service")
	}
}

func TestATeammateWhosePlanFailedIsUncoveredWhileTheRestStillShow(t *testing.T) {
	t.Parallel()
	svc := teamPlanService(theTeam, plansByOwner{
		due:    map[ids.UUID][]PlanWork{theReader: {promiseBy(theReader, "Send the proposal")}},
		failed: map[ids.UUID]error{theColleague: errors.New("plan read failed")},
	})
	day, err := svc.Worklist(meetingPrepReader(), scopeTeam, "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	if planRowOwners(day)["Send the proposal"] != theReader {
		t.Fatal("one teammate's failure took the others' promises off the page")
	}
	if _, missing := planSourceMissing(day); missing {
		t.Fatal("a partly read team was reported as the whole source unavailable")
	}
	read := map[ids.UUID]bool{}
	for _, member := range day.PlanCoverage.Members {
		read[ids.UUID(member.UserId)] = member.Read
	}
	if !read[theReader] || read[theColleague] {
		t.Fatalf("coverage must mark only the failed teammate unread: %v", read)
	}
}

func TestATeamWhosePlansAllFailedNamesTheSourceUnavailable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		want crmcontracts.WorklistSourceUnavailableReason
	}{
		{errors.New("plan read failed"), crmcontracts.WorklistSourceUnavailableReasonFailed},
		{apperrors.ErrPermissionDenied, crmcontracts.WorklistSourceUnavailableReasonWithheld},
	} {
		svc := teamPlanService(theTeam, plansByOwner{failed: map[ids.UUID]error{theReader: tc.err, theColleague: tc.err}})
		day, err := svc.Worklist(meetingPrepReader(), scopeTeam, "", ids.UUID{}, 25, "")
		if err != nil {
			t.Fatal(err)
		}
		if reason, missing := planSourceMissing(day); !missing || reason != tc.want {
			t.Errorf("%v: every plan failing must read as %s, not an empty week; got %q", tc.err, tc.want, reason)
		}
	}
}

func TestATeamReadWithNoRosterNamesThePlansUnavailable(t *testing.T) {
	t.Parallel()
	var asked []ids.UUID
	scoped, missing := teamPlanService(teammatesFailing{}, plansByOwner{asked: &asked}).
		readingPlan(meetingPrepReader(), scopeTeam, rankInstant)
	if missing == nil || missing.Reason != crmcontracts.WorklistSourceUnavailableReasonFailed {
		t.Fatalf("a roster that did not answer must leave the plans named failed, got %+v", missing)
	}
	if len(asked) != 0 || len(scoped.planRows) != 0 {
		t.Fatalf("with no roster nobody's plan may be read: asked=%v", asked)
	}
}

func TestACappedRosterSaysThePlanCoverageIsTruncated(t *testing.T) {
	t.Parallel()
	day, err := teamPlanService(cutRoster(theTeam), plansByOwner{}).
		Worklist(meetingPrepReader(), scopeTeam, "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	if day.PlanCoverage == nil || !day.PlanCoverage.Truncated {
		t.Fatalf("teammates past the cap were never asked, and coverage must say so: %+v", day.PlanCoverage)
	}
	for _, reach := range day.Reach {
		if string(reach.Source) == sourceWeeklyCommitment {
			if !reach.MoreAvailable {
				t.Fatal("reach reported the promise lane fully seen past a capped roster")
			}
			return
		}
	}
	t.Fatal("a team read that asked for plans left the promise lane out of reach")
}

func TestASlowTeamReadStopsAtOneLaneBudgetAndNamesWhoWasNotAsked(t *testing.T) {
	t.Parallel()
	var asked []ids.UUID
	svc := teamPlanService(theTeam, plansByOwner{
		due:   map[ids.UUID][]PlanWork{theReader: {promiseBy(theReader, "Send the proposal")}},
		asked: &asked,
	})
	// Every plan read costs a whole lane budget, so the first spends the team's.
	svc.now = func() time.Time { return rankInstant.Add(time.Duration(len(asked)) * laneBudget) }
	scoped, missing := svc.readingPlan(meetingPrepReader(), scopeTeam, rankInstant)
	if len(asked) != 1 || asked[0] != theReader {
		t.Fatalf("a spent budget must stop the reads, asked=%v", asked)
	}
	if missing != nil || len(scoped.planRows) != 1 {
		t.Fatalf("the plan read in time must still show: missing=%+v rows=%d", missing, len(scoped.planRows))
	}
	read := map[ids.UUID]bool{}
	for _, member := range scoped.planCoverage.Members {
		read[ids.UUID(member.UserId)] = member.Read
	}
	if !read[theReader] || read[theColleague] {
		t.Fatalf("the teammate never asked must read as unread: %v", read)
	}
}

func TestATeamWithMixedRefusalsNamesThePlansFailedWhateverTheRosterOrder(t *testing.T) {
	t.Parallel()
	denied, broken := apperrors.ErrPermissionDenied, errors.New("plan read failed")
	for _, failed := range []map[ids.UUID]error{
		{theReader: denied, theColleague: broken},
		{theReader: broken, theColleague: denied},
	} {
		day, err := teamPlanService(theTeam, plansByOwner{failed: failed}).
			Worklist(meetingPrepReader(), scopeTeam, "", ids.UUID{}, 25, "")
		if err != nil {
			t.Fatal(err)
		}
		if reason, _ := planSourceMissing(day); reason != crmcontracts.WorklistSourceUnavailableReasonFailed {
			t.Errorf("withheld is for a team that refused every read; a failed one must win, got %q", reason)
		}
	}
}

func TestTheAllScopeStillWithholdsWeeklyPlans(t *testing.T) {
	t.Parallel()
	var asked []ids.UUID
	svc := teamPlanService(theTeam, plansByOwner{
		due:   map[ids.UUID][]PlanWork{theColleague: {promiseBy(theColleague, "Call the buyer back")}},
		asked: &asked,
	})
	day, err := svc.Worklist(meetingPrepReader(), scopeAll, "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	if reason, _ := planSourceMissing(day); reason != crmcontracts.WorklistSourceUnavailableReasonWithheld {
		t.Fatalf("all has no roster to read plans across, so it must stay withheld; got %q", reason)
	}
	if len(asked) != 0 || day.PlanCoverage != nil {
		t.Fatalf("all read plans it has no policy for: asked=%v coverage=%+v", asked, day.PlanCoverage)
	}
}
