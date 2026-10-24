// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestReportingFailureKeepsItsWrittenLanguageAndRetryIntent(t *testing.T) {
	for _, tc := range []struct{ language, explanation string }{
		{"en", "Report generation failed. Retry the run."},
		{"de", "Die Berichtserstellung ist fehlgeschlagen. Starte den Lauf erneut."},
		{"vi", "Tạo báo cáo thất bại. Hãy chạy lại."},
	} {
		t.Run(tc.language, func(t *testing.T) {
			f := reportingBusiness(t)
			integration.SetBaseLanguage(f.human, t, f.env.Pool, tc.language)
			catalog, err := f.service.Catalog(f.human)
			if err != nil {
				t.Fatal(err)
			}
			expected := reportingDefinitionWords[reportingBookingsWon].In(textlang.Lang(tc.language))
			if len(catalog.Metrics) == 0 || catalog.Metrics[0].Definition != expected {
				t.Fatalf("%s catalog not localized: %+v", tc.language, catalog)
			}
			report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Failure language", Audience: "workspace", Selection: f.selection()})
			if err != nil {
				t.Fatal(err)
			}
			run, err := f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, "language-proof")
			if err != nil {
				t.Fatal(err)
			}
			db := InstallationDB(f.env.Pool)
			failed := reporting.NewService(db, metricEvaluator{forecast: newForecastStore(db)}, cancelledPublicationAuthority{reportingAuthority{users: identity.NewService(f.env.Pool)}}, reportingCalendar, func() time.Time { return f.at }).WithBaseLanguage(identity.BaseLanguageForRecord)
			if err := failed.Sweep(principal.SystemActing(f.human, "system:language-proof")); err != nil {
				t.Fatal(err)
			}
			integration.SetBaseLanguage(f.human, t, f.env.Pool, "en")
			current, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
			if err != nil {
				t.Fatal(err)
			}
			if current.Reason == nil || *current.Reason != tc.explanation || current.Status != "pending" || current.Attempt != 1 || current.EditionId != nil || !current.IntendedDueAt.Equal(run.IntendedDueAt) {
				t.Fatalf("changed recorded failure or retry intent: %+v", current)
			}
		})
	}
}

func TestUnconfiguredReportingFrameworkUsesRecordLanguage(t *testing.T) {
	for _, tc := range []struct{ language, reason string }{
		{"en", "Not configured"}, {"de", "Nicht eingerichtet"}, {"vi", "Chưa thiết lập"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			env := setupForecast(t)
			ctx := reportingActor(env)
			integration.SetBaseLanguage(ctx, t, env.Pool, tc.language)
			service := newReportingService(env.Pool, func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) })
			framework, err := service.GetFramework(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if framework.Definition.Reason != tc.reason || framework.Revision != 0 {
				t.Fatalf("unexpected initial framework: %+v", framework)
			}
		})
	}
}

func TestReportingCatchUpKeepsItsWrittenLanguage(t *testing.T) {
	for _, tc := range []struct{ language, prefix string }{
		{"en", "Skipped 2 overdue periods"}, {"de", "2 überfällige Zeiträume"}, {"vi", "Đã bỏ qua 2 kỳ quá hạn"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			f := reportingBusiness(t)
			integration.SetBaseLanguage(f.human, t, f.env.Pool, tc.language)
			if _, err := f.env.owner.Exec(t.Context(), `INSERT INTO retention_policy(object_type,retain_days,action,enabled) VALUES('report_edition',730,'erase',true)`); err != nil {
				t.Fatal(err)
			}
			report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Catch up", Audience: "workspace", Selection: f.selection()})
			if err != nil {
				t.Fatal(err)
			}
			schedule, err := f.service.CreateSchedule(f.human, ids.UUID(report.Id), crmcontracts.ReportingScheduleInput{Frequency: "weekly", Day: 1, LocalTime: "09:00", Enabled: true, ReportRevision: report.Revision})
			if err != nil {
				t.Fatal(err)
			}
			f.at = schedule.NextDueAt.AddDate(0, 0, 13*7).Add(12 * time.Hour)
			if err := f.service.Sweep(f.writer); err != nil {
				t.Fatal(err)
			}
			integration.SetBaseLanguage(f.human, t, f.env.Pool, "en")
			runs, err := f.service.ListExecutions(f.human, ids.UUID(report.Id), nil, 30)
			if err != nil {
				t.Fatal(err)
			}
			var skipped int
			for _, run := range runs.Data {
				if run.Status != "skipped" {
					continue
				}
				skipped++
				if run.Reason == nil || !strings.HasPrefix(*run.Reason, tc.prefix) || !strings.Contains(*run.Reason, schedule.NextDueAt.Format(time.RFC3339)) {
					t.Fatalf("catch-up reason lost language or start: %+v", run)
				}
			}
			if skipped != 1 || len(runs.Data) != 13 {
				t.Fatalf("catch-up changed: %+v", runs)
			}
		})
	}
}
