// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/colleaguework"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

func readReportingIntroductionsMade(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	var b reportingBindings
	reading := colleaguework.Reading{Fields: sqlUnnarrowed, Label: b.add(reporting.ExplanationCopy(frame).Say(reportingLabelWords.Introduction))}
	return readColleagueWork(ctx, tx, frame, reportingIntroductionsMade, introductions.Table, &b, colleaguework.IntroductionsMade, reading)
}

func readReportingMeetingsBooked(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	var b reportingBindings
	fields, masked, err := reportingFieldScope(ctx, string(recordTypeActivity), "a", reportingMeetingFields(), &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	reading := colleaguework.Reading{Fields: fields, Label: b.add(reporting.ExplanationCopy(frame).Say(reportingLabelWords.Meeting))}
	facts, coverage, err := readColleagueWork(ctx, tx, frame, reportingMeetingsBookedForColleagues, string(recordTypeActivity), &b, colleaguework.MeetingsBookedForColleagues, reading)
	coverage.Withheld = coverage.Withheld || masked
	return facts, coverage, err
}

// readColleagueWork reads one fact per piece of a figure's work, credited to
// the seat in the scope that did it. Work the reader may not open is not read.
func readColleagueWork(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, source string, b *reportingBindings, figure colleaguework.Figure, reading colleaguework.Reading) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	_, credited, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), figure.Seat, b.arg, unownedIsExcluded)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	reading.Credited = orUnnarrowed(credited)
	reading.From, reading.Until, reading.Limit = b.add(reportingEarliest(frame)), b.add(frame.EvaluatedAt), b.add(reportingFactLimit+1)
	query, err := figure.SQL(ctx, reading, b.arg)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	return readOutcomeFacts(ctx, tx, frame, metric, source, query, *b)
}
