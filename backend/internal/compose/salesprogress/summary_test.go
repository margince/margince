// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/salesassessment"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestTheSummaryLabelsAreWrittenInEveryLanguage(t *testing.T) {
	langcopytest.Census(t, summaryWords)
}

// octoberReading is a month three weeks in: 60,000 closed of a 100,000 goal.
func octoberReading() crmcontracts.SalesProgress {
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	goal, delta, completion := int64(10000000), int64(-4000000), 0.6
	scope := "Sam Okafor"
	progress := crmcontracts.SalesProgress{
		Timezone: "UTC", Currency: "EUR", Cutoff: start.AddDate(0, 0, 20),
		Horizon:      crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 1, 0)},
		ActualWindow: &crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 0, 20)},
		Scope:        crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKindOwner, Label: &scope},
		Goal:         crmcontracts.SalesProgressGoal{Status: crmcontracts.SalesProgressGoalStatusSet, AmountMinor: &goal, MonthsSet: 1, MonthsNeeded: 1},
		Closed:       okAmount(6000000), Planned: okAmount(2500000),
		DeltaMinor: &delta, Completion: &completion,
		Categories: crmcontracts.SalesProgressCategories{ProductMinor: 4000000, ServiceMinor: 2000000},
	}
	progress.Period.Kind = crmcontracts.SalesProgressPeriodKindMonth
	return progress
}

func summaryCell(t *testing.T, in salesassessment.Input, id string) salesassessment.CellIn {
	t.Helper()
	for _, cell := range in.Cells {
		if cell.ID == id {
			return cell
		}
	}
	t.Fatalf("the summary holds no %s cell among %+v", id, in.Cells)
	return salesassessment.CellIn{}
}

func hasSummaryCell(in salesassessment.Input, id string) bool {
	for _, cell := range in.Cells {
		if cell.ID == id {
			return true
		}
	}
	return false
}

func TestSummaryReadsThePageFiguresInMajorUnits(t *testing.T) {
	in := SummaryOf(octoberReading(), time.UTC, langcopy.For("en"))
	if in.View != salesassessment.ViewProgress || in.Frame.Scope != "Sam Okafor" || in.Frame.Window != "2026-10-01 to 2026-11-01" || in.Frame.Today != "2026-10-21" {
		t.Fatalf("frame = %+v under view %q", in.Frame, in.View)
	}
	for id, want := range map[string]string{
		"progress:goal": "100000.00 EUR", "progress:closed": "60000.00 EUR", "progress:missing": "40000.00 EUR",
		"progress:completion": "60 percent", "progress:planned": "25000.00 EUR",
		"progress:product": "40000.00 EUR", "progress:service": "20000.00 EUR",
	} {
		if cell := summaryCell(t, in, id); cell.Value != want || cell.Coverage != "ok" {
			t.Errorf("%s = %q under %q, want %q under ok", id, cell.Value, cell.Coverage, want)
		}
	}
	if closed := summaryCell(t, in, "progress:closed"); closed.Window != "2026-10-01 to 2026-10-21" {
		t.Errorf("closed revenue covers %q, want the days up to the cutoff", closed.Window)
	}
	if hasSummaryCell(in, "progress:over") || hasSummaryCell(in, "progress:unresolved") {
		t.Error("a reading behind its goal with every sale typed carries no surplus and no untyped revenue")
	}
}

func TestSummaryLeavesOutWhatThePageDoesNotShow(t *testing.T) {
	unset := octoberReading()
	unset.Goal = crmcontracts.SalesProgressGoal{Status: crmcontracts.SalesProgressGoalStatusMissing, MonthsNeeded: 1}
	unset.DeltaMinor, unset.Completion = nil, nil
	in := SummaryOf(unset, time.UTC, langcopy.For("en"))
	for _, id := range []string{"progress:goal", "progress:missing", "progress:completion"} {
		if hasSummaryCell(in, id) {
			t.Errorf("a reading with no goal carries %s", id)
		}
	}
	ahead := octoberReading()
	ahead.ActualWindow = nil
	ahead.Closed = crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusNotStarted}
	ahead.Planned = crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusWithheld}
	in = SummaryOf(ahead, time.UTC, langcopy.For("en"))
	if hasSummaryCell(in, "progress:closed") || hasSummaryCell(in, "progress:planned") || hasSummaryCell(in, "progress:product") {
		t.Errorf("a period that has not started, with its open deals withheld, carries %+v", in.Cells)
	}
}

func TestSummaryReadsAGoalSetForSomeMonthsAsPartial(t *testing.T) {
	year := octoberReading()
	subtotal := int64(30000000)
	year.Goal = crmcontracts.SalesProgressGoal{Status: crmcontracts.SalesProgressGoalStatusIncomplete, AmountMinor: &subtotal, MonthsSet: 3, MonthsNeeded: 12}
	goal := summaryCell(t, SummaryOf(year, time.UTC, langcopy.For("de")), "progress:goal")
	if goal.Coverage != "partial" || goal.CoverageDetail != "3/12 Monate des Zeitraums haben ein Ziel" || goal.Label != "Umsatzziel" {
		t.Fatalf("goal = %+v, want a partial figure that says how many months it counts, in the reader's language", goal)
	}
}

func TestSummaryNamesEachTeamMemberUpToItsBound(t *testing.T) {
	team := octoberReading()
	goal := int64(5000000)
	for range summaryMembers + 3 {
		team.Members = append(team.Members, crmcontracts.SalesProgressMember{
			Name: "Dana Reyes", Closed: okAmount(1200000),
			Goal: crmcontracts.SalesProgressGoal{Status: crmcontracts.SalesProgressGoalStatusSet, AmountMinor: &goal},
		})
	}
	in := SummaryOf(team, time.UTC, langcopy.For("en"))
	if cell := summaryCell(t, in, "member:1:closed"); cell.Label != "Closed revenue · Dana Reyes" || cell.Value != "12000.00 EUR" {
		t.Fatalf("member cell = %+v", cell)
	}
	if !hasSummaryCell(in, "member:12:goal") || hasSummaryCell(in, "member:13:closed") {
		t.Fatal("the summary reads twelve members one by one and no more")
	}
}

// A written summary rests on the cells production builds, so the reply a
// model gives over them passes the production reader.
func TestAProgressSummaryOverTheseCellsIsKept(t *testing.T) {
	in := SummaryOf(octoberReading(), time.UTC, langcopy.For("en"))
	salesassessment.DisplayFigures(&in, "en")
	written, err := salesassessment.Parse(`{"paragraphs":[[{"text":"You've closed {{progress:closed}} of the {{progress:goal}} goal, so {{progress:missing}} is still missing.","kind":"observation","cells":["progress:closed","progress:goal","progress:missing"]},{"text":"Open deals worth {{progress:planned}} are expected to close in the period.","kind":"observation","cells":["progress:planned"]}]],"action_ids":[],"next_decision":null,"confidence":"medium"}`, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.Sentences) != 2 || written.Next != nil {
		t.Fatalf("kept %+v, want both sentences and no decision", written)
	}
}
