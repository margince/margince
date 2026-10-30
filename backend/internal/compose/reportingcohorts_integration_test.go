// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stageCohortFixture moves deals through the production deal writer on a
// clock the test steps forward.
type stageCohortFixture struct {
	env       *forecastEnv
	at        time.Time
	store     *deals.Store
	writer    context.Context
	won, lost ids.UUID
}

func newStageCohortFixture(t *testing.T) *stageCohortFixture {
	t.Helper()
	e := setupForecast(t)
	f := &stageCohortFixture{env: e, at: clocktest.Now(t).Add(-30 * 24 * time.Hour)}
	f.writer = principal.WithCorrelationID(principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:cohort-fixture"), ids.NewV7())
	f.store = deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).WithClock(func() time.Time { return f.at })
	f.won = e.seedID(t, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Cohort won',20,'won',100)`, e.pipeline)
	f.lost = e.seedID(t, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Cohort lost',21,'lost',0)`, e.pipeline)
	return f
}

// stage names the open stages by position: 0, 1 and 2.
func (f *stageCohortFixture) stage(position int) ids.UUID {
	return f.env.stages[map[int]int{0: 20, 1: 55, 2: 60}[position]]
}

// deal opens a deal in a stage, then makes each move an hour after the last.
func (f *stageCohortFixture) deal(t *testing.T, owner ids.UUID, moves ...ids.UUID) ids.DealID {
	t.Helper()
	user := ids.From[ids.UserKind](owner)
	created, err := f.store.CreateDeal(f.writer, deals.CreateDealInput{Name: "Cohort deal", PipelineID: ids.From[ids.PipelineKind](f.env.pipeline), StageID: ids.From[ids.StageKind](moves[0]), OwnerID: &user, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.DealKind](ids.UUID(created.Id))
	for _, to := range moves[1:] {
		f.at = f.at.Add(time.Hour)
		won, lost := "purchase_order", "price"
		input := deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](to)}
		switch to {
		case f.won:
			input.WonWithoutContractReason = &won
		case f.lost:
			input.LostReason = &lost
		}
		if _, err := f.store.AdvanceDeal(f.writer, id, input); err != nil {
			t.Fatal(err)
		}
	}
	f.at = f.at.Add(time.Hour)
	return id
}

func (f *stageCohortFixture) evaluate(t *testing.T, service *reporting.Service, scope crmcontracts.ReportingScope, interval crmcontracts.ReportingWindow) crmcontracts.ReportingEvaluation {
	t.Helper()
	selection := crmcontracts.ReportingSelection{Scope: scope, PipelineId: ptrUUID(f.env.pipeline), Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingStageConversion}, Blocks: []crmcontracts.ReportingBlockKind{reportingStageCohort}}
	result, err := service.Evaluate(reportingActor(f.env), selection)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// cohortRow is one stage point read as outcome and entry counts by key.
type cohortRow struct {
	observations int64
	value        *float64
	outcomes     map[string]int64
	entries      map[string]int64
}

func cohortRows(t *testing.T, result crmcontracts.ReportingEvaluation) map[string]cohortRow {
	t.Helper()
	if len(result.Charts) != 1 {
		t.Fatalf("got %d charts, want the stage cohort alone", len(result.Charts))
	}
	rows := map[string]cohortRow{}
	for _, point := range result.Charts[0].Points {
		row := cohortRow{observations: *point.Observations, value: point.Value, outcomes: map[string]int64{}, entries: map[string]int64{}}
		var counted int64
		for _, outcome := range *point.Outcomes {
			row.outcomes[outcome.Key] = outcome.Count
			counted += outcome.Count
		}
		if counted != row.observations {
			t.Fatalf("stage %s: outcomes add up to %d of %d members", point.Label, counted, row.observations)
		}
		if point.Entries != nil {
			for _, entry := range *point.Entries {
				row.entries[entry.Key] = entry.Count
			}
		}
		rows[point.Key] = row
	}
	return rows
}

// Each deal is followed from its first entry into a stage in the interval to
// its next move. A deal still in the stage is unresolved and stays out of the
// rate; a move recorded without the stage order is counted apart.
func TestStageCohortReconcilesRegressionRepeatReopenSkipAndUnresolvedEntries(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1, s2 := f.stage(0), f.stage(1), f.stage(2)
	rep1, rep3 := f.env.Rep1, f.env.Rep3

	repeat := f.deal(t, rep1, s1, s0)
	start := f.at
	f.deal(t, rep1, s0, s1)
	f.deal(t, rep1, s0, s2, f.lost)
	f.deal(t, rep1, s0, s1, s0)
	f.deal(t, rep1, s0, f.lost, s1, f.won)
	f.deal(t, rep1, s0)
	f.deal(t, rep3, s0, s1)
	unknown := f.deal(t, rep1, s0, s1)
	if _, err := f.store.AdvanceDeal(f.writer, repeat, deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](s1)}); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	if _, err := f.store.AdvanceDeal(f.writer, repeat, deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](s2)}); err != nil {
		t.Fatal(err)
	}
	// A move written before the stage order was kept carries no positions.
	if _, err := f.env.owner.Exec(context.Background(), `UPDATE deal_stage_history SET from_position_at_change=NULL, to_position_at_change=NULL
		WHERE deal_id=$1 AND to_stage_id=$2`, unknown.UUID, s1); err != nil {
		t.Fatal(err)
	}
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at.Add(time.Hour)}
	at := f.at.Add(2 * time.Hour)
	service := newReportingService(f.env.Pool, func() time.Time { return at })

	result := f.evaluate(t, service, crmcontracts.ReportingScope{Kind: "workspace"}, interval)
	rows := cohortRows(t, result)

	first := rows[s0.String()]
	if first.observations != 7 || first.outcomes[cohortAdvanced] != 4 || first.outcomes[cohortLost] != 1 || first.outcomes[cohortUnresolved] != 1 || first.outcomes[cohortHistoryPartial] != 1 || first.value == nil || *first.value != 80 {
		t.Fatalf("first stage: %+v, want 4 advanced and 1 lost of 5 resolved, 1 open, 1 unknown, rate 80", first)
	}
	second := rows[s1.String()]
	if second.observations != 6 || second.outcomes[cohortAdvanced] != 1 || second.outcomes[cohortWon] != 1 || second.outcomes[cohortRegressed] != 1 || second.outcomes[cohortUnresolved] != 3 || second.value != nil {
		t.Fatalf("second stage: %+v, want 1 advanced, 1 won, 1 back, 3 open, no rate under 5 resolved", second)
	}
	if second.entries[cohortEntryFirst] != 5 || second.entries[cohortEntryRepeat] != 1 || second.entries[cohortEntryReopened] != 1 {
		t.Fatalf("second stage entries: %+v, want 5 first, 1 repeat, 1 reopened", second.entries)
	}
	third := rows[s2.String()]
	if third.observations != 2 || third.outcomes[cohortLost] != 1 || third.outcomes[cohortUnresolved] != 1 {
		t.Fatalf("skipped-to stage: %+v, want the skip's loss and the repeat's open entry", third)
	}
	metric := result.Metrics[0]
	if metric.Numerator == nil || *metric.Numerator != 6 || *metric.Denominator != 9 || metric.Coverage.Status != reportingPartial {
		t.Fatalf("stage conversion: %+v, want 6 of 9 resolved, partial for the unknown direction", metric)
	}

	evidence, err := service.Evidence(reportingActor(f.env), result.Selection, reportingStageConversion, reportingInterval, s0.String()+":"+cohortAdvanced, nil, nil, 50,
		reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 4 {
		t.Fatalf("the first stage's advanced count names %d deals, want 4", len(evidence.Rows))
	}
}

// Reordering stages today leaves an old move's direction as it was recorded.
// The deal that moved from the first stage to the second still advanced.
func TestStageCohortKeepsAMovesDirectionAfterTheStagesAreReordered(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1, s2 := f.stage(0), f.stage(1), f.stage(2)
	start := f.at
	for range 5 {
		f.deal(t, f.env.Rep1, s0, s1)
	}
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at}
	at := f.at.Add(time.Hour)
	service := newReportingService(f.env.Pool, func() time.Time { return at })
	before := cohortRows(t, f.evaluate(t, service, crmcontracts.ReportingScope{Kind: "workspace"}, interval))[s0.String()]

	if _, err := f.store.ReorderStages(f.writer, ids.From[ids.PipelineKind](f.env.pipeline), []ids.UUID{s1, s0, s2, f.won, f.lost}, nil); err != nil {
		t.Fatal(err)
	}
	after := cohortRows(t, f.evaluate(t, service, crmcontracts.ReportingScope{Kind: "workspace"}, interval))[s0.String()]

	if before.outcomes[cohortAdvanced] != 5 || after.outcomes[cohortAdvanced] != 5 || after.outcomes[cohortRegressed] != 0 {
		t.Fatalf("before %+v, after reorder %+v: want 5 advanced both times", before.outcomes, after.outcomes)
	}
}

// An entry is credited to the owner the move recorded. Reassigning the deal
// afterwards moves no entry to the new owner.
func TestStageCohortCreditsTheOwnerAtEntryAfterAReassignment(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1 := f.stage(0), f.stage(1)
	start := f.at
	moved := f.deal(t, f.env.Rep1, s0, s1)
	newOwner := ids.From[ids.UserKind](f.env.Rep3)
	if _, err := f.store.UpdateDeal(f.writer, moved, deals.UpdateDealInput{OwnerID: &newOwner}); err != nil {
		t.Fatal(err)
	}
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at}
	at := f.at.Add(time.Hour)
	service := newReportingService(f.env.Pool, func() time.Time { return at })

	original := cohortRows(t, f.evaluate(t, service, crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep1)}, interval))
	current := cohortRows(t, f.evaluate(t, service, crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep3)}, interval))

	if original[s0.String()].outcomes[cohortAdvanced] != 1 || len(current) != 0 {
		t.Fatalf("entry owner's cohort %+v, new owner's cohort %+v: want the entry with its first owner only", original, current)
	}
}

// The lead cohort follows the leads that arrived in the interval to their
// furthest outcome by the cutoff. Each is credited to the owner it arrived
// with, through a reassignment and an erasure of the contact it became.
func TestLeadCohortFollowsArrivalsToTheirOutcomeForTheOwnerTheyArrivedWith(t *testing.T) {
	e := setupForecast(t)
	start := clocktest.Now(t).Add(-time.Minute)
	ctx := reportingActor(e)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	ctx = principal.WithActor(ctx, actor)
	store := contacts.NewStore(InstallationDB(e.Pool))
	rep1, rep3 := ids.From[ids.UserKind](e.Rep1), ids.From[ids.UserKind](e.Rep3)
	lead := func(owner *ids.UserID) ids.LeadID {
		name := "Cohort lead"
		created, _, err := store.CreateLead(ctx, contacts.CreateLeadInput{FullName: &name, Status: "new", OwnerID: owner, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.LeadKind](ids.UUID(created.Id))
	}
	promote := func(id ids.LeadID) crmcontracts.Contact {
		contact, _, err := store.PromoteLead(ctx, id, contacts.PromoteLeadInput{Trigger: "human_qualify"})
		if err != nil {
			t.Fatal(err)
		}
		return contact
	}
	disqualify := func(id ids.LeadID) {
		if _, err := store.DisqualifyLead(ctx, id, contacts.DisqualifyLeadInput{}); err != nil {
			t.Fatal(err)
		}
	}

	reassigned := lead(&rep1)
	if _, err := store.UpdateLead(ctx, reassigned, contacts.UpdateLeadInput{OwnerID: &rep3}); err != nil {
		t.Fatal(err)
	}
	promote(reassigned)
	erased := promote(lead(&rep1))
	disqualify(lead(&rep1))
	disqualify(lead(&rep1))
	lead(&rep1)
	handedOn := lead(&rep1)
	handoff, err := store.SubmitHandoff(ctx, contacts.NewSDRHandoff{LeadID: &handedOn.UUID})
	if err != nil {
		t.Fatal(err)
	}
	deal := e.seedOpenDeal(t, "Accepted from a lead", 55, &e.Rep1, nil, nil)
	if err := store.DecideHandoff(ctx, handoff, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &deal}); err != nil {
		t.Fatal(err)
	}
	claimed := lead(nil)
	if _, err := store.UpdateLead(ctx, claimed, contacts.UpdateLeadInput{OwnerID: &rep1}); err != nil {
		t.Fatal(err)
	}
	if err := privacy.NewEraser(InstallationDB(e.Pool)).EraseContact(ctx, ids.UUID(erased.Id), "subject request"); err != nil {
		t.Fatal(err)
	}

	at := clocktest.Now(t).Add(time.Minute)
	service := newReportingService(e.Pool, func() time.Time { return at })
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: at}
	evaluate := func(owner ids.UUID) crmcontracts.ReportingEvaluation {
		selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(owner)}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingLeadCohortOutcomes}, Blocks: []crmcontracts.ReportingBlockKind{reportingLeadCohort}}
		result, err := service.Evaluate(ctx, selection)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	arrivedWith := evaluate(e.Rep1)
	point := arrivedWith.Charts[0].Points[0]
	counts := map[string]int64{}
	for _, outcome := range *point.Outcomes {
		counts[outcome.Key] = outcome.Count
	}
	if *point.Observations != 7 || counts[cohortAccepted] != 1 || counts[cohortQualified] != 2 || counts[cohortDisqualified] != 2 || counts[cohortUnresolved] != 2 {
		t.Fatalf("arrivals %d with outcomes %+v, want 7: 1 accepted, 2 qualified, 2 disqualified, 2 open", *point.Observations, counts)
	}
	metric := arrivedWith.Metrics[0]
	if metric.Value == nil || *metric.Value != 60 || *metric.Numerator != 3 || *metric.Denominator != 5 {
		t.Fatalf("lead cohort: %+v, want 3 of 5 resolved, the 2 open left out", metric)
	}
	if current := evaluate(e.Rep3); len(current.Charts[0].Points) != 1 || *current.Charts[0].Points[0].Observations != 0 {
		t.Fatalf("the reassigned lead's new owner got %+v, want no arrivals", current.Charts[0].Points)
	}
}
