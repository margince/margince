// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A phase move a won deal caused names that cause on the project page. It is a
// code and the deal behind it, so a client can word it. A move a person made
// names none. The win runs through the real deals store.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pursuedThenWon moves the fixture project to pursuing by hand and then wins
// its deal, so its history holds a manual move and an automatic one.
func pursuedThenWon(t *testing.T, e *Env, name string) closeWonFixture {
	t.Helper()
	f := seedCloseWonFixture(t, e, name)
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), f.project, projects.AdvanceProjectPhaseInput{
		ToPhase: projects.PhasePursuing,
	}); err != nil {
		t.Fatalf("move the project to pursuing: %v", err)
	}
	if _, err := e.Deals.AdvanceDeal(e.Admin(), f.deal, wonInput(f.wonStage)); err != nil {
		t.Fatalf("win the deal: %v", err)
	}
	return f
}

func TestAWonDealNamesItselfAsTheCauseOfTheDeliveryMove(t *testing.T) {
	e := Setup(t)
	f := pursuedThenWon(t, e, "ERP replacement")

	rows := phaseHistoryOf(t, e, f.project)
	if len(rows) != 3 {
		t.Fatalf("history has %d rows, want birth, pursuing and delivering", len(rows))
	}
	for i, row := range rows[:2] {
		if row.Cause != nil {
			t.Errorf("row %d (to %s) cause = %+v, want none: a person made that move", i, row.ToPhase, row.Cause)
		}
	}
	won := rows[2]
	if won.ToPhase != projects.PhaseDelivering || won.Cause == nil {
		t.Fatalf("last row = %s with cause %+v, want delivering with a cause", won.ToPhase, won.Cause)
	}
	if won.Cause.Kind != crmcontracts.Project360PhaseCauseKindDealWon {
		t.Errorf("cause kind = %q, want deal_won", won.Cause.Kind)
	}
	if won.Cause.DealId == nil || ids.UUID(*won.Cause.DealId) != f.deal.UUID {
		t.Errorf("cause deal_id = %v, want the won deal %s", won.Cause.DealId, f.deal.UUID)
	}
	if won.Cause.DealName == nil || *won.Cause.DealName != "ERP replacement phase one" {
		t.Errorf("cause deal_name = %v, want the won deal's name", won.Cause.DealName)
	}
	if won.Reason != nil {
		t.Errorf("reason = %q, want none: the cause is a code, never stored prose", *won.Reason)
	}
}

// A reader without the deal grant keeps the history and the cause kind, with
// the deal unnamed.
func TestTheCauseLeavesTheDealUnnamedForAReaderWithoutTheDealGrant(t *testing.T) {
	e := Setup(t)
	f := pursuedThenWon(t, e, "Hidden pursuit")

	page, err := project360Service(e, clocktest.Now(t).UTC()).
		Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, withoutGrant(project360RepPerms, "deal")), f.project)
	if err != nil {
		t.Fatalf("assemble without the deal grant: %v", err)
	}
	if page.PhaseHistory == nil {
		t.Fatal("phase_history omitted, want it kept with the deal unnamed")
	}
	won := page.PhaseHistory.Data[len(page.PhaseHistory.Data)-1]
	if won.Cause == nil || won.Cause.Kind != crmcontracts.Project360PhaseCauseKindDealWon {
		t.Fatalf("cause = %+v, want deal_won", won.Cause)
	}
	if won.Cause.DealId != nil || won.Cause.DealName != nil {
		t.Errorf("cause names deal %v %v, want both null for a reader without the deal grant",
			won.Cause.DealId, won.Cause.DealName)
	}
}
