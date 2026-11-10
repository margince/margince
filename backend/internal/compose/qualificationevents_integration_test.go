// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestQualificationCreditCountsFirstEventForItsOwnerRatherThanArrivalMonth(t *testing.T) {
	e := setupForecast(t)
	ctx := reportingActor(e)
	actor, _ := principal.Actor(ctx)
	for _, object := range []string{"lead", "relationship", "custom_field"} {
		actor.Permissions.Objects[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	ctx = principal.WithActor(ctx, actor)
	at := clocktest.Now(t).AddDate(0, 2, 0)
	ds := deals.NewStore(InstallationDB(e.Pool), DealsInstallation())
	cs := contacts.NewStore(InstallationDB(e.Pool)).SetClock(func() time.Time { return at }).WithDealOpener(leadDealOpener{deals: ds})
	owner := ids.From[ids.UserKind](e.Rep1)
	lead, _, err := cs.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Qualified buyer"), OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	out, err := cs.QualifyLead(ctx, id, contacts.PromoteLeadInput{Trigger: "human_qualify", Deal: &contacts.QualifyDealInput{PipelineID: &e.pipeline, StageID: new(e.stages[20])}})
	if err != nil {
		t.Fatal(err)
	}
	if out.DealID == nil {
		t.Fatal("qualification did not create a deal")
	}
	if _, err := ds.ArchiveDeal(ctx, ids.From[ids.DealKind](*out.DealID), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.DemoteLead(ctx, id, "Qualification withdrawn"); err != nil {
		t.Fatal(err)
	}
	other := ids.From[ids.UserKind](e.Rep3)
	if _, err := cs.UpdateLead(ctx, id, contacts.UpdateLeadInput{OwnerID: &other}); err != nil {
		t.Fatal(err)
	}
	at = at.Add(time.Hour)
	if _, err := cs.QualifyLead(ctx, id, contacts.PromoteLeadInput{Trigger: "human_qualify", Deal: &contacts.QualifyDealInput{PipelineID: &e.pipeline, StageID: new(e.stages[20])}}); err != nil {
		t.Fatal(err)
	}
	service := newReportingService(e.Pool, func() time.Time { return at.Add(time.Hour) })
	span := crmcontracts.ReportingWindow{StartAt: at.Add(-2 * time.Hour), EndAt: at.Add(time.Hour)}
	for _, test := range []struct {
		owner ids.UUID
		want  float64
	}{{e.Rep1, 1}, {e.Rep3, 0}} {
		selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(test.owner)}, Period: "custom", Interval: &span, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingLeadsQualified}, Blocks: []crmcontracts.ReportingBlockKind{}}
		result, err := service.Evaluate(ctx, selection)
		if err != nil {
			t.Fatal(err)
		}
		rows := sectionsEvidence(ctx, t, service, result, reportingLeadsQualified, "interval", "")
		if len(rows) != int(test.want) {
			t.Errorf("evidence has %d rows, want %v", len(rows), test.want)
		}
		got := sectionsValue(t, result, reportingLeadsQualified)
		if got != test.want {
			t.Errorf("owner %s: %v, want %v", test.owner, got, test.want)
		}
	}
	if err := InstallationDB(e.Pool).Tx(ctx, func(tx pgx.Tx) error {
		team, err := readSdrTeamQualifications(ctx, tx, []ids.UUID{e.Rep1, e.Rep3}, sdrTeamWindow{from: span.StartAt, to: span.EndAt}, nil, at.Add(time.Hour))
		if err != nil {
			return err
		}
		if team.owners[e.Rep1].qualified != 1 || team.owners[e.Rep3].qualified != 0 || team.unattributed != 0 {
			t.Errorf("team credit: %+v", team)
		}
		// Older audit records have no owner evidence; the current promotion writer cannot emit that shape.
		_, err = storekit.Audit(ctx, tx, "promote", "lead", id.UUID, map[string]any{"status": "engaged"}, map[string]any{"status": "promoted", "qualified_deal_id": *out.DealID, "promoted_at": at.Add(-90 * time.Minute)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []crmcontracts.ReportingScope{{Kind: "workspace"}, {Kind: "owner", Id: ptrUUID(e.Rep1)}} {
		selection := crmcontracts.ReportingSelection{Scope: scope, Period: "custom", Interval: &span, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingLeadsQualified, reportingLeadsPromoted}, Blocks: []crmcontracts.ReportingBlockKind{}}
		result, err := service.Evaluate(ctx, selection)
		if err != nil {
			t.Fatal(err)
		}
		want := 1.0
		if scope.Kind == "owner" {
			want = 0
			if sectionsReading(t, result, reportingLeadsQualified).Coverage.Status != "partial" {
				t.Fatalf("unknown scoped credit: %+v", sectionsReading(t, result, reportingLeadsQualified))
			}
		}
		if got := sectionsValue(t, result, reportingLeadsQualified); got != want {
			t.Errorf("%s: %v, want %v", scope.Kind, got, want)
		}
		if got := sectionsValue(t, result, reportingLeadsPromoted); got != 0 {
			t.Errorf("arrival cohort changed: %v", got)
		}
	}

	unowned, _, err := cs.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Unowned qualified buyer"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.QualifyLead(ctx, ids.From[ids.LeadKind](ids.UUID(unowned.Id)), contacts.PromoteLeadInput{Trigger: "human_qualify", Deal: &contacts.QualifyDealInput{PipelineID: &e.pipeline, StageID: new(e.stages[20])}}); err != nil {
		t.Fatal(err)
	}
	result, err := service.Evaluate(ctx, crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &span, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingLeadsQualified}, Blocks: []crmcontracts.ReportingBlockKind{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := sectionsValue(t, result, reportingLeadsQualified); got != 2 {
		t.Fatalf("workspace count with unowned qualification: %v", got)
	}
	if err := InstallationDB(e.Pool).Tx(ctx, func(tx pgx.Tx) error {
		team, err := readSdrTeamQualifications(ctx, tx, []ids.UUID{e.Rep1, e.Rep3}, sdrTeamWindow{from: span.StartAt, to: span.EndAt}, nil, at.Add(time.Hour))
		if err == nil && (team.unattributed != 1 || len(team.owners) != 0) {
			t.Errorf("unknown credit assigned to a member: %+v", team)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
