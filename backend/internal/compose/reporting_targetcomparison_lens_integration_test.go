// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The comparison answers for the population asked for, under the reader's own
// row scope and the individual-performance switch.

func (q targetQuarter) ownerSelection() crmcontracts.ReportingSelection {
	selection := q.selection()
	selection.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(q.env.Rep1)}
	return selection
}

func (q targetQuarter) teamSelection() crmcontracts.ReportingSelection {
	selection := q.selection()
	selection.Scope = crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(q.env.Team1)}
	return selection
}

func TestARepOnTheirOwnRowsComparesTheirOwnQuarter(t *testing.T) {
	q := seedTargetQuarter(t)
	q.openDeal(t, q.env.Rep3, "A colleague's deal", 5_000_000)
	q.quarterTarget(t, crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(q.env.Rep1)}, 80_000_000)
	rep := reportingTieredReader(q.human, principal.RowScopeOwn)
	_, got := q.comparisonFor(rep, t, q.ownerSelection())
	if got.Status != "compared" || *got.OpenPipeline != 136_000_000 {
		t.Fatalf("own quarter %+v, want the rep's own 1.36m compared: a row scope that hides nothing they asked for narrows nothing", got)
	}
}

// maskedLead reads every deal and sees the amount only of the deals its team
// may change.
func (q targetQuarter) maskedLead() context.Context {
	lead := reportingTieredReader(q.human, principal.RowScopeTeam)
	actor, _ := principal.Actor(lead)
	actor.Permissions.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}}
	return principal.WithActor(lead, actor)
}

func TestAReaderWhoCannotSeeADealsAmountGetsNoComparisonOfItsScope(t *testing.T) {
	q := seedTargetQuarter(t)
	q.openDeal(t, q.env.Rep3, "Another team's deal", 5_000_000)
	q.quarterTarget(t, crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(q.env.Rep1)}, 80_000_000)
	_, got := q.comparisonFor(q.maskedLead(), t, q.selection())
	if got.Status != "partial_visibility" || got.Gap != nil {
		t.Fatalf("company quarter with another team's amount hidden %+v, want partial visibility and no gap", got)
	}
	if _, got := q.comparisonFor(q.maskedLead(), t, q.ownerSelection()); got.Status != "compared" {
		t.Fatalf("the same reader's own quarter %+v, want it compared: nothing they asked for is hidden", got)
	}
}

func TestACallBelowWhatIsWonGivesNeitherReachedNorShort(t *testing.T) {
	q := seedTargetQuarter(t)
	calls := forecasting.NewStore(InstallationDB(q.env.Pool))
	if err := calls.InTx(q.human, func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, q.at)
		if err != nil {
			return err
		}
		_, err = calls.RecordCallTx(ctx, tx, forecasting.NewCall{Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace}, AmountMinor: 30_000_000, Currency: base, Note: "Cut"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, got := q.comparison(t, q.selection())
	if got.Status != "call_below_won" || got.Gap != nil || got.Remaining != nil {
		t.Fatalf("a 300k call beside 420k won %+v, want the call refused with no figure", got)
	}
}

func TestWithIndividualPerformanceOffAComparisonNeverShowsOneColleague(t *testing.T) {
	q := seedTargetQuarter(t)
	q.quarterTarget(t, crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(q.env.Rep1)}, 80_000_000)
	q.quarterTarget(t, crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(q.env.Team1)}, 90_000_000)
	colleagues := q.env.addMembers(t, q.env.Team1, "colleague1@forecast.test", "colleague2@forecast.test")
	q.openDeal(t, colleagues[0], "First colleague's deal", 5_000_000)
	q.env.showIndividualPerformance(t, false)

	if _, got := q.comparison(t, q.ownerSelection()); got.Status != "compared" {
		t.Fatalf("own quarter %+v, want it compared: the switch hides colleagues, not the reader", got)
	}
	if _, got := q.comparison(t, q.teamSelection()); got.Status != "colleague_withheld" || got.Gap != nil || got.OpenPipeline != nil {
		t.Fatalf("a team of the reader and one selling colleague %+v, want it withheld with no figure", got)
	}
	q.openDeal(t, colleagues[1], "Second colleague's deal", 5_000_000)
	_, got := q.comparison(t, q.teamSelection())
	if got.Status != "compared" || *got.Target != 90_000_000 || *got.OpenPipeline != 146_000_000 {
		t.Fatalf("a team of the reader and two selling colleagues %+v, want the team target compared over 1.46m open", got)
	}
}

// Three colleagues are behind the quarter, and one of them behind its open
// pipeline. The open total minus the reader's own is that colleague's pipeline.
func TestWithIndividualPerformanceOffOneColleaguesOpenPipelineIsWithheld(t *testing.T) {
	q := seedTargetQuarter(t)
	q.quarterTarget(t, crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(q.env.Team1)}, 90_000_000)
	colleagues := q.env.addMembers(t, q.env.Team1, "colleague1@forecast.test", "colleague2@forecast.test", "colleague3@forecast.test")
	q.openDeal(t, colleagues[0], "First colleague's deal", 5_000_000)
	q.win(t, q.openDeal(t, colleagues[1], "Second colleague's win", 5_000_000))
	q.win(t, q.openDeal(t, colleagues[2], "Third colleague's win", 5_000_000))
	q.env.showIndividualPerformance(t, false)
	if _, got := q.comparison(t, q.teamSelection()); got.Status != "colleague_withheld" || got.OpenPipeline != nil {
		t.Fatalf("team quarter %+v, want it withheld: one colleague makes up the open pipeline beside the reader", got)
	}
}
