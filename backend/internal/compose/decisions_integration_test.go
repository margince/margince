// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A decision recorded twice with one text on one deal is one open task. Its
// outcome closes that task with the note and changes nothing on the deal.
// After that, the same decision starts a new task.
func TestADecisionIsOneTaskUntilItsOutcomeIsRecorded(t *testing.T) {
	e := setupForecast(t)
	start := clocktest.Now(t).UTC().Truncate(time.Second)
	_, _, open := seedInitiativeDeals(t, e, start)
	ctx := initiativeManager(e)
	seam := newSalesDecisions(InstallationDB(e.Pool))
	expected := "Budget owner named"
	in := crmcontracts.SalesDecisionInput{
		Decision:       "Call the buyer about the budget",
		DealId:         openapi_types.UUID(open["Linden press"]),
		OwnerId:        openapi_types.UUID(e.Rep1),
		ReviewOn:       openapi_types.Date{Time: start.AddDate(0, 0, 7)},
		ExpectedResult: &expected,
	}
	first, created, err := seam.Record(ctx, in)
	if err != nil {
		t.Fatalf("recording the decision: %v", err)
	}
	if !created || first.Status != crmcontracts.SalesDecisionStatusOpen || first.DealId != in.DealId {
		t.Fatalf("the first record gave %+v, created=%v", first, created)
	}
	again, created, err := seam.Record(ctx, in)
	if err != nil {
		t.Fatalf("recording the decision again: %v", err)
	}
	if created || again.Id != first.Id {
		t.Fatalf("a second record of the open decision gave %+v, created=%v; want the first task back", again, created)
	}

	// The same words on another deal, or handed to another owner, are another
	// decision.
	elsewhere := in
	elsewhere.DealId = openapi_types.UUID(open["Tern portal"])
	if other, created, err := seam.Record(ctx, elsewhere); err != nil || !created || other.Id == first.Id {
		t.Fatalf("the decision on another deal gave %+v, created=%v, err=%v; want a task of its own", other, created, err)
	}
	handed := in
	handed.OwnerId = openapi_types.UUID(e.Rep3)
	if other, created, err := seam.Record(ctx, handed); err != nil || !created || other.Id == first.Id {
		t.Fatalf("the decision handed to another owner gave %+v, created=%v, err=%v; want a task of its own", other, created, err)
	}

	deal := open["Linden press"]
	var before time.Time
	if err := e.Pool.QueryRow(context.Background(), `SELECT updated_at FROM deal WHERE id = $1`, deal).Scan(&before); err != nil {
		t.Fatal(err)
	}
	listed, err := seam.List(ctx, decisionFilter{deal: &deal})
	if err != nil {
		t.Fatalf("listing the deal's decisions: %v", err)
	}
	if len(listed) != 2 || listed[0].ExpectedResult == nil || *listed[0].ExpectedResult != expected {
		t.Fatalf("the deal's decisions are %+v, want its two open decisions with their expected result", listed)
	}

	done, err := seam.Outcome(ctx, ids.UUID(first.Id), "Buyer met; the budget owner is named")
	if err != nil {
		t.Fatalf("recording the outcome: %v", err)
	}
	if done.Status != crmcontracts.SalesDecisionStatusDone || done.Result == nil || *done.Result != "Buyer met; the budget owner is named" || done.ExpectedResult == nil || *done.ExpectedResult != expected {
		t.Fatalf("the outcome left %+v", done)
	}
	var status string
	var after time.Time
	if err := e.Pool.QueryRow(context.Background(), `SELECT status, updated_at FROM deal WHERE id = $1`, deal).Scan(&status, &after); err != nil {
		t.Fatal(err)
	}
	if status != "open" || !after.Equal(before) {
		t.Fatalf("recording an outcome left the deal %q, updated %v then %v; want it untouched", status, before, after)
	}
	if _, err := seam.Outcome(ctx, ids.UUID(first.Id), "again"); err == nil {
		t.Fatal("a second outcome on a closed decision was accepted")
	}

	third, created, err := seam.Record(ctx, in)
	if err != nil {
		t.Fatalf("recording the decision after its outcome: %v", err)
	}
	if !created || third.Id == first.Id {
		t.Fatalf("after the outcome the same decision gave %+v, created=%v; want a new task", third, created)
	}
	openOnly := "open"
	listed, err = seam.List(ctx, decisionFilter{deal: &deal, status: &openOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Id != third.Id {
		t.Fatalf("the open decisions are %+v, want the new task first and the handed one", listed)
	}
}

// A review's selection keeps the decisions on its deals: the deals its scope
// owns, of its industry. A selection that holds no deal answers no decision,
// never the unnarrowed list.
func TestTheDecisionListFollowsTheReviewSelection(t *testing.T) {
	e := setupForecast(t)
	start := clocktest.Now(t).UTC().Truncate(time.Second)
	_, _, open := seedInitiativeDeals(t, e, start)
	ctx := initiativeManager(e)
	seam := newSalesDecisions(InstallationDB(e.Pool))
	for _, name := range []string{"Linden press", "Tern portal"} {
		if _, _, err := seam.Record(ctx, crmcontracts.SalesDecisionInput{
			Decision: "Name the budget owner", DealId: openapi_types.UUID(open[name]),
			OwnerId: openapi_types.UUID(e.Rep1), ReviewOn: openapi_types.Date{Time: start.AddDate(0, 0, 7)},
		}); err != nil {
			t.Fatalf("recording the decision on %s: %v", name, err)
		}
	}
	decided := func(selection crmcontracts.ReportingSelection) []openapi_types.UUID {
		t.Helper()
		listed, err := seam.List(ctx, decisionFilter{selection: &selection})
		if err != nil {
			t.Fatalf("listing under %+v: %v", selection, err)
		}
		deals := []openapi_types.UUID{}
		for _, decision := range listed {
			deals = append(deals, decision.DealId)
		}
		return deals
	}
	manufacturing, retail := "manufacturing", "retail"
	rep1, rep3 := openapi_types.UUID(e.Rep1), openapi_types.UUID(e.Rep3)

	if got := decided(crmcontracts.ReportingSelection{Industry: &manufacturing}); len(got) != 1 || got[0] != openapi_types.UUID(open["Linden press"]) {
		t.Errorf("the manufacturing review lists decisions on %v, want only Linden press's", got)
	}
	if got := decided(crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: &rep1}}); len(got) != 2 {
		t.Errorf("Rep1's review lists decisions on %v, want both of Rep1's deals", got)
	}
	if got := decided(crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: &rep3}}); len(got) != 0 {
		t.Errorf("Rep3's review lists decisions on %v, want none: Rep3 owns no deal", got)
	}
	if got := decided(crmcontracts.ReportingSelection{Industry: &retail}); len(got) != 0 {
		t.Errorf("a review of an industry with no deal lists decisions on %v, want none", got)
	}
}
