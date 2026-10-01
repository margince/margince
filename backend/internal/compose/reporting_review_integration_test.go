// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingCompanyUsesCurrentOwnersForLegacyAndDepartedBookings(t *testing.T) {
	f := reportingBusiness(t)
	// NULL attribution is the pre-upgrade history shape; the closes themselves use the real writer.
	if _, err := f.env.owner.Exec(context.Background(), "UPDATE deal_stage_history SET owner_id_at_change=NULL WHERE owner_id_at_change=$1", f.env.Rep1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.owner.Exec(context.Background(), "UPDATE app_user SET archived_at=now() WHERE id=$1", f.env.Rep3); err != nil {
		t.Fatal(err)
	}
	selection := f.selection()
	selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"owner_attainment"}
	result, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics[0].Value == nil || *result.Metrics[0].Value != 21600000 {
		t.Fatalf("bookings lost: %+v", result.Metrics)
	}
	if result.Metrics[0].Coverage.Status != "ok" {
		t.Fatalf("obsolete history hid current ownership: %+v", result.Metrics[0].Coverage)
	}
	found := false
	for _, point := range result.Charts[0].Points {
		if point.Key == f.env.Rep1.String() {
			found = point.Value != nil && *point.Value == 10800000
		}
		if point.Key == f.env.Rep3.String() && point.Target != nil {
			t.Fatal("departed owner has a live target")
		}
	}
	if !found {
		t.Fatalf("current owner missing: %+v", result.Charts[0])
	}
}

func TestReportingNarrowEditionStripsRosterAndTargetsWithoutHiddenFacts(t *testing.T) {
	f := reportingBusiness(t)
	f.at = time.Date(2026, 12, 3, 12, 0, 0, 0, time.UTC)
	selection := f.selection()
	selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"owner_attainment"}
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Empty monthly review", Audience: "workspace", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), 0, "empty-month")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Sweep(principal.SystemActing(f.human, "system:empty-month")); err != nil {
		t.Fatal(err)
	}
	completed, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("publication: %+v %v", completed, err)
	}
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.RowScope = principal.RowScopeOwn
	narrowed := principal.WithActor(f.human, actor)
	edition, err := f.service.GetEdition(narrowed, ids.UUID(*completed.EditionId))
	if err != nil {
		t.Fatal(err)
	}
	if !edition.Withheld || len(edition.Evaluation.Context.MemberIds) != 0 {
		t.Fatalf("roster survived projection: %+v", edition.Evaluation.Context)
	}
	for _, chart := range edition.Evaluation.Charts {
		if len(chart.Points) != 0 || chart.AllocatedTarget != nil || chart.AllocationDifference != nil {
			t.Fatalf("peer targets survived: %+v", chart)
		}
	}
	for _, metric := range edition.Evaluation.Metrics {
		if metric.Target != nil || metric.Attainment != nil {
			t.Fatalf("target survived: %+v", metric)
		}
	}
}

func TestReportingLatestRevisionFreezeReplayKeepsItsOriginalRevision(t *testing.T) {
	f := reportingBusiness(t)
	input := crmcontracts.ReportingReportInput{Name: "Original", Audience: "private", Selection: f.selection()}
	report, err := f.service.CreateReport(f.human, input)
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.service.Freeze(f.human, ids.UUID(report.Id), 0, "latest-intent")
	if err != nil {
		t.Fatal(err)
	}
	input.Name = "Revised"
	if _, err := f.service.UpdateReport(f.human, ids.UUID(report.Id), report.Version, input); err != nil {
		t.Fatal(err)
	}
	repeat, err := f.service.Freeze(f.human, ids.UUID(report.Id), 0, "latest-intent")
	if err != nil || first.Id != repeat.Id || first.ReportRevision != repeat.ReportRevision {
		t.Fatalf("revision replay changed intent: %+v %+v %v", first, repeat, err)
	}
}
