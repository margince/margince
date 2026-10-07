// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type weeklyNumericEvaluator struct{}

func (weeklyNumericEvaluator) Measure(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID, start, end, at time.Time) (weekly.NumericResult, error) {
	var out weekly.NumericResult
	wireID := openapi_types.UUID(id)
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKind(kind), Id: &wireID}, Period: "custom", Interval: &crmcontracts.ReportingWindow{StartAt: start, EndAt: end}, TargetBasis: reportingMonthContext, CloseWindow: "all_open"}
	frame, err := reportingContext(ctx, tx, selection, crmcontracts.ReportingFramework{}, at)
	if err != nil {
		return out, err
	}
	out.Summary = crmcontracts.WeeklyNumericSummary{Version: "analytics", Timezone: frame.Timezone, Currency: frame.Currency, Interval: frame.Interval, EvaluatedAt: at}
	for _, spec := range reportingMetrics {
		if spec.definition.Id != reportingBookingsWon && spec.definition.Id != reportingMeetingsHeld {
			continue
		}
		out.Summary.Version += ":" + string(spec.definition.Id) + "@" + spec.definition.Version
		coverage := reportingGap(reportingUnavailable, "Source access is unavailable")
		var facts []reporting.Fact
		if auth.Allows(ctx, spec.object, principal.ActionRead) {
			facts, coverage, err = spec.read(ctx, tx, frame, crmcontracts.ReportingFramework{}, metricEvaluator{})
			if err != nil {
				return out, err
			}
		} else {
			coverage.Withheld = true
		}
		coverage, err = reportingHistoryCoverage(ctx, tx, frame, spec.definition.Id, coverage)
		if err != nil {
			return out, err
		}
		periodFacts := metricFacts(facts, spec.definition.Id, reportingInterval)
		metric := projectReportingMetric(spec.definition, frame, facts, coverage)
		if spec.definition.Id == reportingBookingsWon {
			out.Won = len(periodFacts)
			out.Summary.BookingsCoverage = metric.Coverage
			if metric.Value != nil {
				value := int64(*metric.Value)
				out.Summary.WonMinor = &value
			}
		} else {
			out.Held = len(periodFacts)
			out.Summary.MeetingsCoverage = metric.Coverage
			out.WithNextStep, err = weeklyConfirmedFollowup(ctx, tx, periodFacts, end)
			if err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

func weeklyConfirmedFollowup(ctx context.Context, tx pgx.Tx, facts []reporting.Fact, end time.Time) (int, error) {
	if len(facts) == 0 {
		return 0, nil
	}
	var b reportingBindings
	meetingIDs := make([]ids.UUID, 0, len(facts))
	for _, fact := range facts {
		meetingIDs = append(meetingIDs, fact.SourceID)
	}
	visible, err := auth.ActivityContentClause(ctx, "task", b.arg)
	if err != nil {
		return 0, err
	}
	if visible == "" {
		visible = sqlUnnarrowed
	}
	query := "SELECT count(DISTINCT task.source_activity_id) FROM activity task JOIN activity meeting ON meeting.id=task.source_activity_id WHERE task.source_activity_id=ANY(" + b.add(meetingIDs) + "::uuid[]) AND task.kind='task' AND task.archived_at IS NULL AND task.created_at>=meeting.occurred_at AND task.created_at<" + b.add(end) + " AND " + visible
	var count int
	err = tx.QueryRow(ctx, query, b.values...).Scan(&count)
	return count, err
}
