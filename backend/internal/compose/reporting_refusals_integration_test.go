// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReportingScheduleAndFreezeRefusalsNameTheirField(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Refusals", Audience: "private", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	valid := crmcontracts.ReportingScheduleInput{Frequency: "weekly", Day: 1, LocalTime: "09:00", ReportRevision: report.Revision}
	cases := []struct {
		name   string
		field  string
		change func(*crmcontracts.ReportingScheduleInput)
	}{
		{"retention not configured", "enabled", func(in *crmcontracts.ReportingScheduleInput) { in.Enabled = true }},
		{"day", "day", func(in *crmcontracts.ReportingScheduleInput) { in.Day = 9 }},
		{"frequency", "frequency", func(in *crmcontracts.ReportingScheduleInput) { in.Frequency = "yearly" }},
		{"time shape", "local_time", func(in *crmcontracts.ReportingScheduleInput) { in.LocalTime = "9" }},
		{"hour", "local_time", func(in *crmcontracts.ReportingScheduleInput) { in.LocalTime = "25:00" }},
		{"minute", "local_time", func(in *crmcontracts.ReportingScheduleInput) { in.LocalTime = "09:61" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rule := valid
			test.change(&rule)
			_, err := f.service.CreateSchedule(f.human, ids.UUID(report.Id), rule)
			requireRefusedField(t, err, test.field)
		})
	}
	_, err = f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, strings.Repeat("k", 129))
	requireRefusedField(t, err, "Idempotency-Key")
}

func TestReportingTargetRevisionNamesTheFieldItChanged(t *testing.T) {
	f := reportingBusiness(t)
	cases := []struct {
		name   string
		field  string
		change func(*crmcontracts.ReportingTargetInput)
	}{
		{"metric", "metric", func(in *crmcontracts.ReportingTargetInput) { in.Metric = "qualified_pipeline_created" }},
		{"scope", "scope", func(in *crmcontracts.ReportingTargetInput) {
			in.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep1)}
		}},
		{"pipeline", "pipeline_id", func(in *crmcontracts.ReportingTargetInput) { in.PipelineId = nil }},
		{"period", "period_start", func(in *crmcontracts.ReportingTargetInput) {
			in.PeriodStart.Time = in.PeriodStart.AddDate(0, 1, 0)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := f.target.Definition
			test.change(&changed)
			_, err := f.service.UpdateTarget(f.human, ids.UUID(f.target.Id), f.target.Version, changed)
			requireRefusedField(t, err, test.field)
		})
	}
}
