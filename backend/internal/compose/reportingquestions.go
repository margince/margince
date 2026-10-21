// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maxQuestionSources bounds the records one kept answer rests on. Retention,
// holds and erasure reach a kept answer only through these records, so an
// answer over more of them is not kept at all.
const maxQuestionSources = 10000

// questionCardRows is the rows a question card shows when it names no limit,
// the live card's default.
const questionCardRows = 100

// reportingQuestions answers question cards for sent editions through the
// same analytics engine the live card asks.
type reportingQuestions struct {
	floor analyticsquery.Floor
	names attention.Names
}

var _ reporting.Questions = reportingQuestions{}

// questionQuery is the question as the live card asks it: over the report's
// scope, which managed teams leaves unnamed as the reader's own default.
func questionQuery(question crmcontracts.ReportingCardQuestion, scope crmcontracts.ReportingScope) analyticsquery.Query {
	wire := crmcontracts.AnalyticsQuery{Entity: question.Entity, GroupBy: question.GroupBy, Measures: question.Measures, Limit: question.Limit}
	if wire.Limit == nil {
		rows := questionCardRows
		wire.Limit = &rows
	}
	q := queryFromWire(wire)
	if scope.Kind != "managed_teams" {
		q.ScopeKind = string(scope.Kind)
		if scope.Id != nil {
			q.ScopeID = scope.Id.String()
		}
	}
	return q
}

// unanswerable says the question was refused for this seat, as opposed to
// failing. A refused question has no answer to keep or to show.
func unanswerable(err error) bool {
	var refusal *analyticsquery.RefusalError
	return errors.As(err, &refusal) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound)
}

// Capture implements reporting.Questions.
func (q reportingQuestions) Capture(ctx context.Context, tx pgx.Tx, question crmcontracts.ReportingCardQuestion, scope crmcontracts.ReportingScope) (reporting.CapturedQuestion, error) {
	out := reporting.CapturedQuestion{Entity: question.Entity}
	query := questionQuery(question, scope)
	answer, err := RunAnalyticsQuery(ctx, tx, query, q.floor)
	if unanswerable(err) {
		out.Reason = crmcontracts.ReportingEditionQuestionReasonUnanswered
		return out, nil
	}
	if err != nil {
		return out, err
	}
	sourceType, records, err := analyticsSources(ctx, tx, query, nil)
	if unanswerable(err) {
		out.Reason = crmcontracts.ReportingEditionQuestionReasonUnanswered
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if len(records) > maxQuestionSources {
		out.Reason = crmcontracts.ReportingEditionQuestionReasonTooManyRecords
		return out, nil
	}
	rowOf := map[string]int{}
	for i, row := range answer.Rows {
		if row[withheldColumn] != true {
			rowOf[groupKey(query, row)] = i
		}
	}
	out.Sources = make([][]reporting.QuestionSource, len(answer.Rows))
	for _, record := range records {
		i, served := rowOf[groupKey(query, record)]
		id, err := ids.Parse(fmt.Sprint(record["id"]))
		if !served || err != nil {
			continue
		}
		out.Sources[i] = append(out.Sources[i], reporting.QuestionSource{Type: sourceType, ID: id})
	}
	out.Answer = crmcontracts.AnalyticsAnswer{
		Columns: answer.Columns, Rows: answer.Rows, Withheld: answer.Withheld, TotalSafe: answer.TotalSafe,
		SchemaVersion: answer.SchemaVersion, PopulationNarrowed: populationNarrowedWire(answer.PopulationNarrowed),
	}
	return out, nil
}

// Reveal implements reporting.Questions. The reader must be able to ask the
// question today. A row stays only while they may read every record behind it.
// Any other row goes the way a floored row does: kept in place with every
// value blanked, so the row count says nothing.
func (q reportingQuestions) Reveal(ctx context.Context, tx pgx.Tx, question crmcontracts.ReportingCardQuestion, scope crmcontracts.ReportingScope, kept reporting.KeptQuestion) (crmcontracts.AnalyticsAnswer, bool, error) {
	query := questionQuery(question, scope)
	only := []string{}
	for _, records := range kept.Sources {
		for _, record := range records {
			only = append(only, record.ID.String())
		}
	}
	_, readable, err := analyticsSources(ctx, tx, query, only)
	if unanswerable(err) {
		return crmcontracts.AnalyticsAnswer{}, false, nil
	}
	if err != nil {
		return crmcontracts.AnalyticsAnswer{}, false, err
	}
	visible := make(map[string]bool, len(readable))
	for _, record := range readable {
		visible[fmt.Sprint(record["id"])] = true
	}
	out := kept.Answer
	out.Rows = make([]map[string]any, len(kept.Answer.Rows))
	for i, row := range kept.Answer.Rows {
		out.Rows[i] = row
		if row[withheldColumn] == true || everyRecordVisible(kept.Sources, i, visible) {
			continue
		}
		blank := make(map[string]any, len(out.Columns)+1)
		for _, column := range out.Columns {
			blank[column] = nil
		}
		blank[withheldColumn] = true
		out.Rows[i] = blank
		out.Withheld = true
	}
	out.TotalSafe = analyticsquery.TotalIsSafe(out.Withheld)
	out.Labels = idLabelsWire(analyticsIDLabels(ctx, q.names, query, out.Rows))
	return out, true, nil
}

func everyRecordVisible(sources [][]reporting.QuestionSource, row int, visible map[string]bool) bool {
	if row >= len(sources) {
		return true
	}
	for _, record := range sources[row] {
		if !visible[record.ID.String()] {
			return false
		}
	}
	return true
}

// groupKey places a record or an answer row in its group, compared as the
// explanation compares a cell.
func groupKey(q analyticsquery.Query, row map[string]any) string {
	keys := make([]string, len(q.GroupBy))
	for i, name := range q.GroupBy {
		keys[i] = fmt.Sprint(row[name])
	}
	return strings.Join(keys, "\x1f")
}

// analyticsSources lists the records behind a question under the caller's
// grants: the population's read gate, its vocabulary and its scope, as
// RunAnalyticsQuery applies them. It answers one record past the bound when
// the population is larger.
func analyticsSources(ctx context.Context, tx pgx.Tx, query analyticsquery.Query, only []string) (string, []map[string]any, error) {
	schema, specs, err := AnalyticsSchemaIn(ctx, tx)
	if err != nil {
		return "", nil, err
	}
	spec, ok := specs[query.Entity]
	if !ok {
		return "", nil, apperrors.ErrNotFound
	}
	if err := auth.Require(ctx, string(spec.entity), principal.ActionRead); err != nil {
		return "", nil, err
	}
	var narrowed string
	plan, err := analyticsquery.CompileSources(query, schema, analyticsScope(ctx, tx, spec, query, &narrowed), only, maxQuestionSources+1)
	if err != nil {
		return "", nil, err
	}
	frame, err := readReportFrame(ctx, tx)
	if err != nil {
		return "", nil, err
	}
	sql, args, err := bindReportTokens(ctx, frame, plan.SQL, plan.Args)
	if err != nil {
		return "", nil, err
	}
	pgRows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return "", nil, fmt.Errorf("compose: listing an answer's records: %w", err)
	}
	defer pgRows.Close()
	rows, err := scanReportRows(pgRows, plan.Columns)
	if err != nil {
		return "", nil, fmt.Errorf("compose: reading an answer's records: %w", err)
	}
	return string(spec.entity), rows, nil
}
