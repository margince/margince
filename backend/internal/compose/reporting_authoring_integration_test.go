// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
		change func(*crmcontracts.ReportingFrameworkInput)
		want   error
	}{
		{"no reason", func(in *crmcontracts.ReportingFrameworkInput) { in.Reason = "" }, apperrors.ErrInvalidArgument},
		{"unknown template", func(in *crmcontracts.ReportingFrameworkInput) { in.Template = "unknown" }, apperrors.ErrInvalidArgument},
		{"empty stages", func(in *crmcontracts.ReportingFrameworkInput) {
			in.Qualification = []crmcontracts.ReportingQualification{{PipelineId: valid.PipelineId}}
		}, apperrors.ErrInvalidArgument},
		{"duplicate pipeline", func(in *crmcontracts.ReportingFrameworkInput) {
			in.Qualification = []crmcontracts.ReportingQualification{valid, valid}
		}, apperrors.ErrInvalidArgument},
		{"duplicate stage", func(in *crmcontracts.ReportingFrameworkInput) {
			in.Qualification = []crmcontracts.ReportingQualification{{PipelineId: valid.PipelineId, StageIds: []openapi_types.UUID{valid.StageIds[0], valid.StageIds[0]}}}
		}, apperrors.ErrInvalidArgument},
		{"missing stage", func(in *crmcontracts.ReportingFrameworkInput) {
			in.Qualification = []crmcontracts.ReportingQualification{{PipelineId: valid.PipelineId, StageIds: []openapi_types.UUID{openapi_types.UUID(ids.NewV7())}}}
		}, apperrors.ErrNotFound},
		{"owner capture", func(in *crmcontracts.ReportingFrameworkInput) {
			in.CaptureContexts = []crmcontracts.ReportingCaptureContext{{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep1)}}}
		}, apperrors.ErrInvalidArgument},
		{"duplicate capture", func(in *crmcontracts.ReportingFrameworkInput) {
			in.CaptureContexts = []crmcontracts.ReportingCaptureContext{framework.Definition.CaptureContexts[0], framework.Definition.CaptureContexts[0]}
		}, apperrors.ErrInvalidArgument},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := framework.Definition
			test.change(&input)
			if _, err := f.service.PublishFramework(f.human, framework.Version, input); !errors.Is(err, test.want) {
				t.Fatalf("invalid framework: %v", err)
			}
		})
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
		change func(*crmcontracts.ReportingTargetInput)
	}{
		{"negative", func(in *crmcontracts.ReportingTargetInput) { in.Value = -1 }},
		{"inexact", func(in *crmcontracts.ReportingTargetInput) { in.Value = 9007199254740992 }},
		{"no reason", func(in *crmcontracts.ReportingTargetInput) { in.Reason = " " }},
		{"unsupported metric", func(in *crmcontracts.ReportingTargetInput) { in.Metric = "stage_age" }},
		{"partial month", func(in *crmcontracts.ReportingTargetInput) {
			in.PeriodStart.Time = in.PeriodStart.AddDate(0, 0, 1)
		}},
		{"invalid basis", func(in *crmcontracts.ReportingTargetInput) { in.PeriodKind = "unknown" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := f.target.Definition
			test.change(&input)
			if _, err := f.service.CreateTarget(f.human, input); !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Fatalf("invalid target: %v", err)
			}
		})
	}
	changed := f.target.Definition
	changed.Metric = "qualified_pipeline_created"
	if _, err := f.service.UpdateTarget(f.human, ids.UUID(f.target.Id), f.target.Version, changed); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("target identity changed: %v", err)
	}
	saved, err := f.service.GetTarget(f.human, ids.UUID(f.target.Id))
	if err != nil || saved.Definition.Metric != f.target.Definition.Metric || saved.Version != f.target.Version {
		t.Fatalf("invalid update changed target: %+v %v", saved, err)
	}
}
