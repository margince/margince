// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Stages belong to one pipeline, so with none chosen the reading is refused
// and no member is read.
func TestStageCohortWithoutAPipelineIsRefusedBeforeAnyEntryIsRead(t *testing.T) {
	f := newStageCohortFixture(t)
	start := f.at
	f.deal(t, f.env.Rep1, f.stage(0), f.stage(1))
	at := f.at.Add(time.Hour)
	service := newReportingService(f.env.Pool, func() time.Time { return at })
	selection := f.selection(crmcontracts.ReportingScope{Kind: "workspace"}, crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at})
	selection.PipelineId = nil

	result := f.evaluateAs(reportingActor(f.env), t, service, selection)

	metric := result.Metrics[0]
	if metric.Coverage.Status != crmcontracts.ReportingStatusNotConfigured || metric.Coverage.EligibleCount != nil || len(result.Charts[0].Points) != 0 {
		t.Fatalf("stage conversion with no pipeline: %+v with %d points, want not configured and nothing read", metric.Coverage, len(result.Charts[0].Points))
	}
}

// The cohort serves a deal's name, owner, stage, pipeline and outcome, and
// reads no amount or date. A mask on the amount or close date keeps every
// member; a mask on the owner withholds them.
func TestStageCohortKeepsADealWhoseAmountTheReaderMayNotSee(t *testing.T) {
	f := newStageCohortFixture(t)
	start := f.at
	for range 5 {
		f.deal(t, f.env.Rep1, f.stage(0), f.stage(1))
	}
	at := f.at.Add(time.Hour)
	service := newReportingService(f.env.Pool, func() time.Time { return at })
	selection := f.selection(crmcontracts.ReportingScope{Kind: "workspace"}, crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at})
	maskedOn := func(fields ...string) context.Context {
		ctx := reportingActor(f.env)
		actor, _ := principal.Actor(ctx)
		// A reader whose rows are bounded, since an unbounded one reads every field.
		actor.Permissions.RowScope = principal.RowScopeTeam
		for _, field := range fields {
			actor.Permissions.FieldMasks = append(actor.Permissions.FieldMasks, principal.FieldMask{Object: "deal", Field: field, Condition: principal.MaskAlways})
		}
		return principal.WithActor(ctx, actor)
	}

	kept := cohortRows(t, f.evaluateAs(maskedOn("amount_minor", "expected_close_date", "currency"), t, service, selection))[f.stage(0).String()]
	withheld := f.evaluateAs(maskedOn("owner_id"), t, service, selection)

	if kept.observations != 5 || kept.outcomes[cohortAdvanced] != 5 {
		t.Fatalf("with the amount masked the first stage reads %+v, want all 5 deals advanced", kept)
	}
	if rows := cohortRows(t, withheld); len(rows) != 0 || !withheld.Metrics[0].Coverage.Withheld {
		t.Fatalf("with the owner masked the cohort reads %+v (withheld %v), want no member", rows, withheld.Metrics[0].Coverage.Withheld)
	}
}

// Moves written in one instant keep the order they were written in. Each deal
// here enters, leaves and comes back to the first stage at the same clock.
func TestStageCohortOrdersMovesThatShareAnInstantByTheOrderTheyWereWritten(t *testing.T) {
	f := newStageCohortFixture(t)
	start := f.at
	for range 6 {
		user := ids.From[ids.UserKind](f.env.Rep1)
		created, err := f.store.CreateDeal(f.writer, deals.CreateDealInput{Name: "Same instant", PipelineID: ids.From[ids.PipelineKind](f.env.pipeline), StageID: ids.From[ids.StageKind](f.stage(0)), OwnerID: &user, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		for _, to := range []ids.UUID{f.stage(1), f.stage(0)} {
			if _, err := f.store.AdvanceDeal(f.writer, ids.From[ids.DealKind](ids.UUID(created.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](to)}); err != nil {
				t.Fatal(err)
			}
		}
		f.reverseIDOrder(t, ids.UUID(created.Id))
	}
	at := f.at.Add(time.Hour)
	service := newReportingService(f.env.Pool, func() time.Time { return at })
	rows := cohortRows(t, f.evaluate(t, service, crmcontracts.ReportingScope{Kind: "workspace"}, crmcontracts.ReportingWindow{StartAt: start, EndAt: at}))

	first, second := rows[f.stage(0).String()], rows[f.stage(1).String()]
	if first.outcomes[cohortAdvanced] != 6 || second.outcomes[cohortRegressed] != 6 || second.outcomes[cohortUnresolved] != 0 {
		t.Fatalf("first stage %+v, second %+v: want 6 advanced out of the first and 6 back out of the second", first.outcomes, second.outcomes)
	}
}

// reverseIDOrder gives a deal's moves ids that sort against their write order.
// uuidv7 ids minted on two connections in one millisecond can sort that way.
func (f *stageCohortFixture) reverseIDOrder(t *testing.T, deal ids.UUID) {
	t.Helper()
	rows, err := f.env.owner.Query(context.Background(), `SELECT id FROM deal_stage_history WHERE deal_id=$1 ORDER BY write_seq DESC`, deal)
	if err != nil {
		t.Fatal(err)
	}
	written, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range written {
		if _, err := f.env.owner.Exec(context.Background(), `UPDATE deal_stage_history SET id=$2 WHERE id=$1`, id, ids.NewV7()); err != nil {
			t.Fatal(err)
		}
	}
}

// With individual performance off, a stage outcome or a lead outcome that one
// colleague besides the reader makes up withholds the whole reading.
func TestBothCohortsAreWithheldWhenOneColleagueMakesUpAGroup(t *testing.T) {
	f := newStageCohortFixture(t)
	start := f.at
	f.deal(t, f.env.Rep1, f.stage(0))
	f.deal(t, f.env.Rep3, f.stage(0), f.stage(1))
	store := contacts.NewStore(InstallationDB(f.env.Pool))
	rep3 := ids.From[ids.UserKind](f.env.Rep3)
	name := "A colleague's lead"
	ctx := reportingActor(f.env)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	ctx = principal.WithActor(ctx, actor)
	leadFrom := clocktest.Now(t).Add(-time.Minute)
	if _, _, err := store.CreateLead(ctx, contacts.CreateLeadInput{FullName: &name, Status: "new", OwnerID: &rep3, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	f.env.showIndividualPerformance(t, false)
	at := clocktest.Now(t).Add(time.Minute)
	service := newReportingService(f.env.Pool, func() time.Time { return at })

	stages := f.evaluateAs(ctx, t, service, f.selection(crmcontracts.ReportingScope{Kind: "workspace"}, crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at}))
	leadInterval := crmcontracts.ReportingWindow{StartAt: leadFrom, EndAt: at}
	leads := f.evaluateAs(ctx, t, service, crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &leadInterval, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingLeadCohortOutcomes}, Blocks: []crmcontracts.ReportingBlockKind{reportingLeadCohort}})

	for name, result := range map[string]crmcontracts.ReportingEvaluation{"stage": stages, "lead": leads} {
		coverage := result.Metrics[0].Coverage
		if !coverage.Withheld || coverage.Status != reportingUnavailable || result.Metrics[0].Value != nil {
			t.Errorf("%s cohort with one colleague behind a group: %+v, want it withheld", name, coverage)
		}
	}
}

// Arrivals are bounded by the interval; a lead that arrives later is not read.
func TestLeadCohortOfAPastIntervalReadsOnlyItsArrivals(t *testing.T) {
	e := setupForecast(t)
	ctx := reportingActor(e)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	ctx = principal.WithActor(ctx, actor)
	store := contacts.NewStore(InstallationDB(e.Pool))
	rep1 := ids.From[ids.UserKind](e.Rep1)
	past := clocktest.Now(t).Add(-10 * 24 * time.Hour)
	for i := range 8 {
		name := "Arrival"
		lead, _, err := store.CreateLead(ctx, contacts.CreateLeadInput{FullName: &name, Status: "new", OwnerID: &rep1, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		// The lead writer stamps the database's clock, so two arrivals are
		// moved into the past interval.
		if i < 2 {
			if _, err := e.owner.Exec(context.Background(), `UPDATE lead SET created_at=$2 WHERE id=$1`, lead.Id, past); err != nil {
				t.Fatal(err)
			}
		}
	}
	at := clocktest.Now(t).Add(time.Minute)
	service := newReportingService(e.Pool, func() time.Time { return at })
	interval := crmcontracts.ReportingWindow{StartAt: past.Add(-time.Hour), EndAt: past.Add(time.Hour)}
	result, err := service.Evaluate(ctx, crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingLeadCohortOutcomes}, Blocks: []crmcontracts.ReportingBlockKind{reportingLeadCohort}})
	if err != nil {
		t.Fatal(err)
	}

	if eligible := result.Metrics[0].Coverage.EligibleCount; eligible == nil || *eligible != 2 || *result.Charts[0].Points[0].Observations != 2 {
		t.Fatalf("a past interval read %v arrivals, want only its 2", eligible)
	}
}

// A saved revision records each metric's definition version. A metric whose
// definition changed since is withheld with the reason, not read anew.
func TestASavedReportWithholdsACohortWhoseDefinitionChangedSinceItWasSaved(t *testing.T) {
	f := newStageCohortFixture(t)
	start := f.at
	f.deal(t, f.env.Rep1, f.stage(0), f.stage(1))
	at := f.at.Add(time.Hour)
	service := newReportingService(f.env.Pool, func() time.Time { return at })
	human := reportingActor(f.env)
	report, err := service.CreateReport(human, crmcontracts.ReportingReportInput{Name: "Stage conversion", Audience: "private", Selection: f.selection(crmcontracts.ReportingScope{Kind: "workspace"}, crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at})})
	if err != nil {
		t.Fatal(err)
	}
	var recorded string
	if err := f.env.owner.QueryRow(context.Background(), `SELECT metric_versions->>'stage_conversion' FROM report_definition_revision WHERE report_id=$1`, report.Id).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != "1" {
		t.Fatalf("the saved revision recorded stage_conversion version %q, want 1", recorded)
	}
	if fresh, err := service.EvaluateReport(human, ids.UUID(report.Id), nil); err != nil || fresh.Metrics[0].Coverage.Status == crmcontracts.ReportingStatusUnsupported {
		t.Fatalf("a report read under the definition it was saved with: %+v, %v", fresh.Metrics, err)
	}
	// A revision saved under an earlier definition.
	if _, err := f.env.owner.Exec(context.Background(), `UPDATE report_definition_revision SET metric_versions='{"stage_conversion":"0"}' WHERE report_id=$1`, report.Id); err != nil {
		t.Fatal(err)
	}

	result, err := service.EvaluateReport(human, ids.UUID(report.Id), nil)
	if err != nil {
		t.Fatal(err)
	}

	metric := result.Metrics[0]
	if metric.Coverage.Status != crmcontracts.ReportingStatusUnsupported || metric.Coverage.Reason == nil || !strings.Contains(*metric.Coverage.Reason, "changed") || metric.Value != nil || len(result.Charts[0].Points) != 0 {
		t.Fatalf("a revision saved under another definition read %+v with %d points, want it withheld with the reason", metric, len(result.Charts[0].Points))
	}
}

// Routing and capture both record the owner a lead arrived with.
func TestRoutingAndCaptureRecordTheOwnerALeadArrivedWith(t *testing.T) {
	e := integration.Setup(t)
	name := "Unrouted"
	unowned, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Status: "new", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	owner := ids.From[ids.UserKind](e.Rep1)
	decision, err := e.Contacts.RouteLead(e.As(e.Rep1, nil, integration.AdminPerms), ids.From[ids.LeadKind](ids.UUID(unowned.Id)), contacts.RoutingConfig{Owners: []ids.UserID{owner}})
	if err != nil || !decision.Assigned {
		t.Fatalf("routing the lead = %+v, %v, want it assigned", decision, err)
	}
	// A connector captures for the seat that connected it, which owns its leads.
	syncing := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:test", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"lead": {Create: true, Read: true}}, RowScope: principal.RowScopeAll},
	})
	ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(syncing, connector.NormalizedRecord{
		EntityType: "lead", NaturalKey: connector.NaturalKey{SourceSystem: "test", SourceID: "arrival-1"},
		Fields: capture.LeadFields{FullName: "Captured"}, Source: "test:arrival-1", CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id=$1 AND arrival_owner_id=$2`, ids.UUID(unowned.Id), e.Rep1); n != 1 {
		t.Error("a routed lead did not record the owner routing gave it")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id=$1 AND owner_id IS NOT NULL AND arrival_owner_id=owner_id`, ref.ID); n != 1 {
		t.Error("a captured lead did not record the owner it was captured for")
	}
}
