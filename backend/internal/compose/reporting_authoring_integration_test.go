// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingFrameworkRejectsAmbiguousQualificationAndCaptureScopes(t *testing.T) {
	f := reportingBusiness(t)
	framework, err := f.service.GetFramework(f.human)
	if err != nil {
		t.Fatal(err)
	}
	valid := crmcontracts.ReportingQualification{PipelineId: openapi_types.UUID(f.env.pipeline), StageIds: []openapi_types.UUID{openapi_types.UUID(f.env.stages[60])}}
	cases := []struct {
		name   string
		field  string
		change func(*crmcontracts.ReportingFrameworkInput)
	}{
		{"no reason", "reason", func(in *crmcontracts.ReportingFrameworkInput) { in.Reason = "" }},
		{"long reason", "reason", func(in *crmcontracts.ReportingFrameworkInput) { in.Reason = strings.Repeat("r", 1001) }},
		{"unknown template", "template", func(in *crmcontracts.ReportingFrameworkInput) { in.Template = "unknown" }},
		{"too many captures", "capture_contexts", func(in *crmcontracts.ReportingFrameworkInput) {
			in.CaptureContexts = make([]crmcontracts.ReportingCaptureContext, 21)
		}},
		{"empty stages", "qualification", func(in *crmcontracts.ReportingFrameworkInput) {
			in.Qualification = []crmcontracts.ReportingQualification{{PipelineId: valid.PipelineId}}
		}},
		{"duplicate pipeline", "qualification", func(in *crmcontracts.ReportingFrameworkInput) {
			in.Qualification = []crmcontracts.ReportingQualification{valid, valid}
		}},
		{"duplicate stage", "qualification", func(in *crmcontracts.ReportingFrameworkInput) {
			in.Qualification = []crmcontracts.ReportingQualification{{PipelineId: valid.PipelineId, StageIds: []openapi_types.UUID{valid.StageIds[0], valid.StageIds[0]}}}
		}},
		{"owner capture", "capture_contexts", func(in *crmcontracts.ReportingFrameworkInput) {
			in.CaptureContexts = []crmcontracts.ReportingCaptureContext{{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep1)}}}
		}},
		{"duplicate capture", "capture_contexts", func(in *crmcontracts.ReportingFrameworkInput) {
			in.CaptureContexts = []crmcontracts.ReportingCaptureContext{framework.Definition.CaptureContexts[0], framework.Definition.CaptureContexts[0]}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := framework.Definition
			test.change(&input)
			_, err := f.service.PublishFramework(f.human, framework.Version, input)
			requireRefusedField(t, err, test.field)
		})
	}
	missing := framework.Definition
	missing.Qualification = []crmcontracts.ReportingQualification{{PipelineId: valid.PipelineId, StageIds: []openapi_types.UUID{openapi_types.UUID(ids.NewV7())}}}
	if _, err := f.service.PublishFramework(f.human, framework.Version, missing); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("missing stage: %v", err)
	}
	current, err := f.service.GetFramework(f.human)
	if err != nil || current.Version != framework.Version {
		t.Fatalf("invalid framework advanced version: %+v %v", current, err)
	}
}

func TestReportingSharedManagersCanReviseOnlyVisibleReports(t *testing.T) {
	f := reportingBusiness(t)
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.UserID = f.env.Rep3
	actor.ID = "human:" + f.env.Rep3.String()
	actor.TeamIDs = []ids.UUID{f.env.Team1}
	manager := principal.WithActor(f.human, actor)
	for _, audience := range []crmcontracts.ReportingReportInputAudience{"workspace", "team", "private"} {
		input := crmcontracts.ReportingReportInput{Name: string(audience) + " review", Audience: audience, Selection: f.selection()}
		if audience == "team" {
			input.AudienceTeamId = ptrUUID(f.env.Team1)
		}
		report, err := f.service.CreateReport(f.human, input)
		if err != nil {
			t.Fatal(err)
		}
		input.Name = "Revised review"
		revised, err := f.service.UpdateReport(manager, ids.UUID(report.Id), report.Version, input)
		if audience == "private" {
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("private report exposed: %v", err)
			}
			continue
		}
		if err != nil || revised.Revision != 2 {
			t.Fatalf("shared manager revision: %+v %v", revised, err)
		}
		archived, err := f.service.ArchiveReport(manager, ids.UUID(report.Id))
		if err != nil || archived.ArchivedAt == nil {
			t.Fatalf("archive: %+v %v", archived, err)
		}
		replay, err := f.service.ArchiveReport(manager, ids.UUID(report.Id))
		if err != nil || replay.Version != archived.Version {
			t.Fatalf("archive replay changed version: %+v %v", replay, err)
		}
		if _, err := f.service.UpdateReport(manager, ids.UUID(report.Id), archived.Version, input); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("archived report was writable: %v", err)
		}
	}
	for _, name := range []string{"One", "Two"} {
		if _, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: name, Audience: "private", Selection: f.selection()}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.service.ListReports(f.human, nil, 1, false)
	if err != nil || len(first.Data) != 1 || first.NextCursor == nil {
		t.Fatalf("first page: %+v %v", first, err)
	}
	cursor, err := ids.Parse(*first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.service.ListReports(f.human, &cursor, 1, false)
	if err != nil || len(next.Data) != 1 || next.Data[0].Id == first.Data[0].Id {
		t.Fatalf("next page: %+v %v", next, err)
	}
}

func TestReportingTargetsRejectAmbiguousPeriodsAndIdentityChanges(t *testing.T) {
	f := reportingBusiness(t)
	cases := []struct {
		name   string
		field  string
		change func(*crmcontracts.ReportingTargetInput)
	}{
		{"negative", "value", func(in *crmcontracts.ReportingTargetInput) { in.Value = -1 }},
		{"inexact", "value", func(in *crmcontracts.ReportingTargetInput) { in.Value = 9007199254740992 }},
		{"no reason", "reason", func(in *crmcontracts.ReportingTargetInput) { in.Reason = " " }},
		{"unsupported metric", "metric", func(in *crmcontracts.ReportingTargetInput) { in.Metric = "stage_age" }},
		{"partial month", "period_start", func(in *crmcontracts.ReportingTargetInput) {
			in.PeriodStart.Time = in.PeriodStart.AddDate(0, 0, 1)
		}},
		{"invalid basis", "period_kind", func(in *crmcontracts.ReportingTargetInput) { in.PeriodKind = "unknown" }},
		{"owner without an id", "scope", func(in *crmcontracts.ReportingTargetInput) {
			in.Scope = crmcontracts.ReportingScope{Kind: "owner"}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := f.target.Definition
			test.change(&input)
			_, err := f.service.CreateTarget(f.human, input)
			requireRefusedField(t, err, test.field)
		})
	}
	changed := f.target.Definition
	changed.Metric = "qualified_pipeline_created"
	_, err := f.service.UpdateTarget(f.human, ids.UUID(f.target.Id), f.target.Version, changed)
	requireRefusedField(t, err, "metric")
	saved, err := f.service.GetTarget(f.human, ids.UUID(f.target.Id))
	if err != nil || saved.Definition.Metric != f.target.Definition.Metric || saved.Version != f.target.Version {
		t.Fatalf("invalid update changed target: %+v %v", saved, err)
	}
}

func TestReportingTargetsTakeAPipelineOnlyWhereTheFigureFiltersByOne(t *testing.T) {
	f := reportingBusiness(t)
	for _, metric := range []crmcontracts.ReportingMetricID{"leads_promoted", "meetings_held", "accepted_opportunities"} {
		input := f.target.Definition
		input.Metric = metric
		_, err := f.service.CreateTarget(f.human, input)
		requireRefusedField(t, err, "pipeline_id")
	}
	input := f.target.Definition
	input.Metric = "partner_pipeline"
	if _, err := f.service.CreateTarget(f.human, input); err != nil {
		t.Fatalf("partner pipeline takes a pipeline: %v", err)
	}
	input.Metric, input.PipelineId = "leads_promoted", nil
	leads, err := f.service.CreateTarget(f.human, input)
	if err != nil {
		t.Fatalf("lead target without a pipeline: %v", err)
	}
	input.PipelineId = f.target.Definition.PipelineId
	_, err = f.service.UpdateTarget(f.human, ids.UUID(leads.Id), leads.Version, input)
	requireRefusedField(t, err, "pipeline_id")
}

func TestReportingStoredTargetOnAnUnfilteredPipelineStaysRevisableAndRetirable(t *testing.T) {
	f := reportingBusiness(t)
	input := f.target.Definition
	input.Metric, input.PipelineId = "meetings_held", nil
	stored, err := f.service.CreateTarget(f.human, input)
	if err != nil {
		t.Fatal(err)
	}
	// No writer saves such a target, so the stored row is given its pipeline directly.
	pipeline := f.target.Definition.PipelineId
	if _, err := f.env.owner.Exec(f.human, "UPDATE sales_target SET pipeline_id=$1 WHERE id=$2", pipeline, stored.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.owner.Exec(f.human, "UPDATE sales_target_revision SET definition=jsonb_set(definition,'{pipeline_id}',to_jsonb($1::text)) WHERE target_id=$2", pipeline.String(), stored.Id); err != nil {
		t.Fatal(err)
	}
	stored, err = f.service.GetTarget(f.human, ids.UUID(stored.Id))
	if err != nil || stored.Definition.PipelineId == nil || *stored.Definition.PipelineId != *pipeline {
		t.Fatalf("stored target lost its pipeline: %+v %v", stored, err)
	}
	revision := stored.Definition
	revision.Value, revision.Reason = 20, "Revised plan"
	revised, err := f.service.UpdateTarget(f.human, ids.UUID(stored.Id), stored.Version, revision)
	if err != nil {
		t.Fatalf("revise a stored target: %v", err)
	}
	retired := true
	revision.Retired, revision.Reason = &retired, "Plan withdrawn"
	if _, err := f.service.UpdateTarget(f.human, ids.UUID(stored.Id), revised.Version, revision); err != nil {
		t.Fatalf("retire a stored target: %v", err)
	}
	_, err = f.service.CreateTarget(f.human, stored.Definition)
	requireRefusedField(t, err, "pipeline_id")
}

// requireRefusedField holds the 422 shape the contract lists: a refusal that
// names the field to fix, not a bare invalid argument.
func requireRefusedField(t *testing.T, err error, field string) {
	t.Helper()
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 || fault.Fields[0].Field != field {
		t.Fatalf("want a 422 naming %q, got %v (%+v)", field, err, fault)
	}
}
