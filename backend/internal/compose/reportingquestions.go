// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	other, err := countsOtherRecords(ctx, tx, query)
	if err != nil {
		return out, err
	}
	if other {
		out.Reason = crmcontracts.ReportingEditionQuestionReasonCountsOtherRecords
		return out, nil
	}
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

// Reveal implements reporting.Questions. The reader is shown the kept answer
// only when they could ask the question today. Every record behind every
// served row must also still be in their population and in that row's group.
// Anything less withholds the whole answer: a part of it, read beside the
// floor's complements, could describe the rows left out.
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
	groupOf := make(map[string]string, len(readable))
	for _, record := range readable {
		groupOf[fmt.Sprint(record["id"])] = groupKey(query, record)
	}
	for i, row := range kept.Answer.Rows {
		if row[withheldColumn] == true {
			continue
		}
		if !stillGrouped(query, row, sourcesOf(kept.Sources, i), groupOf) {
			return crmcontracts.AnalyticsAnswer{}, false, nil
		}
	}
	out := kept.Answer
	out.Labels = idLabelsWire(analyticsIDLabels(ctx, q.names, query, out.Rows))
	return out, true, nil
}

func sourcesOf(sources [][]reporting.QuestionSource, row int) []reporting.QuestionSource {
	if row >= len(sources) {
		return nil
	}
	return sources[row]
}

// stillGrouped says every record behind a served row is readable and still
// falls in the row's group. A grouped row with no records fails, since nothing
// then vouches for its keys.
func stillGrouped(query analyticsquery.Query, row map[string]any, records []reporting.QuestionSource, groupOf map[string]string) bool {
	if len(records) == 0 {
		return len(query.GroupBy) == 0
	}
	want := groupKey(query, row)
	for _, record := range records {
		if group, readable := groupOf[record.ID.String()]; !readable || group != want {
			return false
		}
	}
	return true
}

// groupKey places a record or an answer row in its group. JSON keeps a null
// apart from any text, which a plain rendering would not.
func groupKey(q analyticsquery.Query, row map[string]any) string {
	keys := make([]any, len(q.GroupBy))
	for i, name := range q.GroupBy {
		keys[i] = row[name]
	}
	raw, err := json.Marshal(keys)
	if err != nil {
		return fmt.Sprint(keys)
	}
	return string(raw)
}

// countsOtherRecords says a measure or grouping reads other records through a
// subquery. Retention, holds and erasure reach a kept answer only through its
// population's own records, so such an answer is not kept.
func countsOtherRecords(ctx context.Context, tx pgx.Tx, query analyticsquery.Query) (bool, error) {
	schema, _, err := AnalyticsSchemaIn(ctx, tx)
	if err != nil {
		return false, err
	}
	entity, ok := schema.Entities[query.Entity]
	if !ok {
		return false, nil
	}
	fields := slices.Clone(query.GroupBy)
	for _, measure := range query.Measures {
		fields = append(fields, measure.Field)
	}
	for _, name := range fields {
		if strings.Contains(strings.ToUpper(entity.Fields[name].Expr), "SELECT") {
			return true, nil
		}
	}
	return false, nil
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
