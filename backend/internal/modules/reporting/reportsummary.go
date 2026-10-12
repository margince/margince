// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func populateReportSummaries(ctx context.Context, tx pgx.Tx, reports []crmcontracts.ReportingReport) error {
	if len(reports) == 0 {
		return nil
	}
	index := map[ids.UUID]int{}
	for i, report := range reports {
		index[ids.UUID(report.Id)] = i
	}
	if auth.Allows(ctx, "report_edition", principal.ActionRead) {
		if err := populateEditionSummary(ctx, tx, reports, index); err != nil {
			return err
		}
	}
	if auth.Allows(ctx, "report_schedule", principal.ActionRead) {
		return populateScheduleSummary(ctx, tx, reports, index)
	}
	return nil
}

func reportSummaryIDs(reports []crmcontracts.ReportingReport) []ids.UUID {
	idsOut := make([]ids.UUID, len(reports))
	for i, report := range reports {
		idsOut[i] = ids.UUID(report.Id)
	}
	return idsOut
}

func populateEditionSummary(ctx context.Context, tx pgx.Tx, reports []crmcontracts.ReportingReport, index map[ids.UUID]int) error {
	var b bindings
	audience, err := audienceClause(ctx, &b)
	if err != nil {
		return err
	}
	source := "(SELECT report_id,owner_id,publication_audience AS audience,publication_team_id AS audience_team_id,captured_at FROM report_edition) d"
	rows, err := tx.Query(ctx, "SELECT report_id,count(*),max(captured_at) FROM "+source+" WHERE report_id=ANY("+b.add(reportSummaryIDs(reports))+") AND "+audience+" GROUP BY report_id", b.values...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for i := range reports {
		zero := 0
		reports[i].EditionCount = &zero
	}
	for rows.Next() {
		var id ids.UUID
		var count int
		var captured time.Time
		if err := rows.Scan(&id, &count, &captured); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			continue
		}
		reports[i].EditionCount = &count
		reports[i].LatestCapturedAt = &captured
	}
	return rows.Err()
}

func populateScheduleSummary(ctx context.Context, tx pgx.Tx, reports []crmcontracts.ReportingReport, index map[ids.UUID]int) error {
	var b bindings
	rows, err := tx.Query(ctx, "SELECT report_id,definition->>'frequency',enabled,next_due_at,last_status FROM report_schedule s WHERE report_id=ANY("+b.add(reportSummaryIDs(reports))+") ORDER BY (SELECT max(intended_due_at) FROM report_execution e WHERE e.schedule_id=s.id) DESC NULLS LAST,id DESC", b.values...)
	if err != nil {
		return err
	}
	defer rows.Close()
	frequencies := map[ids.UUID][]string{}
	for rows.Next() {
		var id ids.UUID
		var frequency string
		var enabled bool
		var due time.Time
		var status *string
		if err := rows.Scan(&id, &frequency, &enabled, &due, &status); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			continue
		}
		if reports[i].LastStatus == nil {
			reports[i].LastStatus = status
		}
		frequencies[id] = append(frequencies[id], frequency)
		if !enabled {
			if reports[i].PausedScheduleCount == nil {
				count := 0
				reports[i].PausedScheduleCount = &count
			}
			*reports[i].PausedScheduleCount++
			continue
		}
		if reports[i].NextDueAt == nil || due.Before(*reports[i].NextDueAt) {
			reports[i].NextDueAt = &due
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for id, parts := range frequencies {
		cadence := strings.Join(parts, ", ")
		reports[index[id]].Cadence = &cadence
	}
	return nil
}
