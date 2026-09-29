// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func reportingReader(pool *pgxpool.Pool) agents.ReportingReader {
	service := newReportingService(pool, time.Now)
	return func(ctx context.Context, in agents.ReportingRead) (agents.ReportingAnswer, error) {
		return readReportingResult(ctx, service, in)
	}
}

func readReportingResult(ctx context.Context, s *reporting.Service, in agents.ReportingRead) (agents.ReportingAnswer, error) {
	cursor, err := reportingToolCursor(in)
	if err != nil {
		return agents.ReportingAnswer{}, err
	}
	limit := in.Limit
	if limit == 0 {
		limit = 30
	}
	if limit < 1 || limit > 100 {
		return agents.ReportingAnswer{}, apperrors.ErrInvalidArgument
	}
	switch in.Mode {
	case "catalog":
		out, err := s.Catalog(ctx)
		return agents.ReportingAnswer{Catalog: &out}, err
	case "evaluate":
		out, err := s.Evaluate(ctx, in.Selection)
		return agents.ReportingAnswer{Evaluation: &out}, err
	case reportingReports:
		out, err := s.ListReports(ctx, cursor, limit, in.Scheduled)
		return agents.ReportingAnswer{Reports: &out}, err
	case "report":
		out, err := s.GetReport(ctx, in.ID)
		return agents.ReportingAnswer{Report: &out}, err
	case reportingEditions:
		out, err := s.ListEditions(ctx, in.ID, cursor, limit)
		return agents.ReportingAnswer{Editions: &out}, err
	case "edition":
		out, err := s.GetEdition(ctx, in.ID)
		return agents.ReportingAnswer{Edition: &out}, err
	case "compare":
		out, err := s.Compare(ctx, in.ID, in.RightID)
		return agents.ReportingAnswer{Comparison: &out}, err
	case "evidence":
		return readReportingEvidence(ctx, s, in, limit)
	default:
		return agents.ReportingAnswer{}, apperrors.ErrInvalidArgument
	}
}

func readReportingEvidence(ctx context.Context, s *reporting.Service, in agents.ReportingRead, limit int) (agents.ReportingAnswer, error) {
	group := ""
	if in.Reference.GroupKey != nil {
		group = *in.Reference.GroupKey
	}
	if !in.ID.IsZero() {
		out, err := s.EditionEvidence(ctx, in.ID, in.Reference.Metric, in.Reference.ContextId, group, in.Reference.Through, in.Cursor, limit)
		return agents.ReportingAnswer{Evidence: &out}, err
	}
	expectation := reporting.EvidenceExpectation{Key: in.EvaluationKey, At: in.EvaluatedAt, FrameworkRevision: in.FrameworkRevision}
	out, err := s.Evidence(ctx, in.Selection, in.Reference.Metric, in.Reference.ContextId, group, in.Reference.Through, in.Cursor, limit, expectation)
	return agents.ReportingAnswer{Evidence: &out}, err
}

func reportingToolCursor(in agents.ReportingRead) (*ids.UUID, error) {
	var cursor *ids.UUID
	if in.Cursor != nil && (in.Mode == reportingReports || in.Mode == reportingEditions) {
		id, err := ids.Parse(*in.Cursor)
		if err != nil {
			return nil, &storekit.MalformedCursorError{}
		}
		cursor = &id
	}
	return cursor, nil
}
