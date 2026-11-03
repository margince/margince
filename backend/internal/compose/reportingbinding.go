// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one resolver a question card is asked through. A live card, a sent
// edition's capture and a reader's look at what an edition kept all ask the
// question this file resolves. So a bound card cannot mean one thing on the
// page and another in the email.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// BindSavedQuestion implements reporting.Questions. The author must be able to see every
// value the saved question filters on, as a reader must to be answered.
// Period filters take the calendar in force now, so a bound `2026` keeps its
// dates.
func (q reportingQuestions) BindSavedQuestion(ctx context.Context, tx pgx.Tx, savedQuestion ids.UUID) (reporting.Binding, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT query FROM report_run WHERE id = $1`, savedQuestion).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return reporting.Binding{}, httperr.Validation("selection.layout", "not_found", "no saved question has that id")
		}
		return reporting.Binding{}, fmt.Errorf("compose: reading a saved question to bind: %w", err)
	}
	var question analyticsquery.Query
	if err := json.Unmarshal(raw, &question); err != nil {
		return reporting.Binding{}, fmt.Errorf("compose: decoding a saved question to bind: %w", err)
	}
	schema, specs, err := AnalyticsSchemaIn(ctx, tx)
	if err != nil {
		return reporting.Binding{}, err
	}
	inspectable, err := mayInspectFilters(ctx, tx, schema, specs, question)
	if err != nil {
		return reporting.Binding{}, err
	}
	if !inspectable {
		return reporting.Binding{}, httperr.Validation("selection.layout", "not_allowed",
			"you may not see everything this saved question filters on, so it cannot be placed on a report")
	}
	frame, err := readReportFrame(ctx, tx)
	if err != nil {
		return reporting.Binding{}, err
	}
	basis := calendarBasisOf(frame)
	question.Filters = analyticsquery.StampBasis(question.Filters, isPeriodField(question.Entity), basis)
	entity := schema.Entities[question.Entity]
	if _, err := analyticsquery.Narrow(entity, analyticsquery.Scope{}, question); err != nil {
		return reporting.Binding{}, err
	}
	if err := question.Validate(schema); err != nil {
		return reporting.Binding{}, err
	}
	return bindingOf(question)
}

// bindingOf is a bound question with the shape every reader of a report sees.
// The calendar it states is the one its period filters keep, which a question
// saved before a move keeps from before it.
func bindingOf(question analyticsquery.Query) (reporting.Binding, error) {
	shape := analyticsquery.Query{Entity: question.Entity, GroupBy: question.GroupBy, Measures: question.Measures, Limit: question.Limit}
	wire, err := wireFromQuery(shape)
	if err != nil {
		return reporting.Binding{}, err
	}
	raw, err := json.Marshal(question)
	if err != nil {
		return reporting.Binding{}, fmt.Errorf("compose: encoding a bound question: %w", err)
	}
	out := reporting.Binding{
		Query:    raw,
		Question: crmcontracts.ReportingCardQuestion{Entity: wire.Entity, GroupBy: wire.GroupBy, Measures: wire.Measures, Limit: wire.Limit},
		Filtered: len(question.Filters) > 0,
	}
	for _, f := range question.Filters {
		if f.Basis != nil {
			out.Basis = &crmcontracts.ReportingPeriodBasis{Timezone: f.Basis.Timezone, FiscalYearStartMonth: f.Basis.FiscalStartMonth}
			break
		}
	}
	return out, nil
}

// calendarBasisOf is the calendar a period is cut in for this answer.
func calendarBasisOf(frame reportFrame) analyticsquery.CalendarBasis {
	return analyticsquery.CalendarBasis{Timezone: frame.Timezone, FiscalStartMonth: frame.FiscalYearStartMonth}
}

// resolveCard is the question a card asks this reader inside its report.
// restricted is true when the reader may not see a value a bound question
// filters on; nothing may then be asked or shown for them.
func resolveCard(ctx context.Context, tx pgx.Tx, card reporting.QuestionCard, scope crmcontracts.ReportingScope) (query analyticsquery.Query, restricted bool, err error) {
	if card.Question.Binding == nil {
		return questionQuery(card.Question, scope), false, nil
	}
	if card.Bound == nil {
		return analyticsquery.Query{}, true, nil
	}
	var bound analyticsquery.Query
	if err := json.Unmarshal(card.Bound, &bound); err != nil {
		return analyticsquery.Query{}, false, fmt.Errorf("compose: decoding a bound question: %w", err)
	}
	schema, specs, err := AnalyticsSchemaIn(ctx, tx)
	if err != nil {
		return analyticsquery.Query{}, false, err
	}
	inspectable, err := mayInspectFilters(ctx, tx, schema, specs, bound)
	if err != nil || !inspectable {
		return analyticsquery.Query{}, !inspectable, err
	}
	report := analyticsquery.Scope{}
	if scope.Kind != "managed_teams" {
		report.Kind = string(scope.Kind)
		if scope.Id != nil {
			report.ID = scope.Id.String()
		}
	}
	query, err = analyticsquery.Narrow(schema.Entities[bound.Entity], report, bound)
	if err != nil {
		return analyticsquery.Query{}, false, err
	}
	if query.Limit == 0 {
		query.Limit = questionCardRows
	}
	return query, false, nil
}

// mayInspectFilters says whether the caller may see every value a question
// names. That is its population and scope, each field it filters on, and each
// owner and record a filter value names. One answer covers every reason, so a
// reader told no cannot tell which value stopped them.
func mayInspectFilters(ctx context.Context, tx pgx.Tx, schema analyticsquery.Schema, specs map[string]reportSpec, q analyticsquery.Query) (bool, error) {
	entity, known := schema.Entities[q.Entity]
	spec, specified := specs[q.Entity]
	if !known || !specified {
		return false, nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false, apperrors.ErrPermissionDenied
	}
	if q.ScopeKind != "" {
		if _, err := resolveAnalyticsScope(ctx, tx, actor, requestedFromQuery(q)); err != nil {
			return false, deniedOrFault(err)
		}
	}
	for _, f := range q.Filters {
		if _, ok := entity.Lookup(f.Field); !ok {
			return false, nil
		}
		id, err := ids.Parse(fmt.Sprint(f.Value))
		if f.Value == nil || err != nil {
			continue
		}
		visible, err := filterValueVisible(ctx, tx, spec, f.Field, id)
		if err != nil || !visible {
			return false, err
		}
	}
	return true, nil
}

// filterValueVisible says whether the caller may see the owner or the record
// an id in a filter names.
func filterValueVisible(ctx context.Context, tx pgx.Tx, spec reportSpec, field string, id ids.UUID) (bool, error) {
	if namesOwner(spec, field) {
		if err := requireMeasurableOwner(ctx, tx, id); err != nil {
			return false, deniedOrFault(err)
		}
		return true, nil
	}
	table, named := filterRecordType(spec, field)
	if !named {
		return true, nil
	}
	return recordReadable(ctx, tx, table, id)
}

// filterRecordType is the record type a filter's value is the id of, read
// from the declarations that gate the reference when it is grouped by.
func filterRecordType(spec reportSpec, field string) (string, bool) {
	if table, named := groupRecordType(spec, field); named {
		return table, true
	}
	expr, ok := spec.filters[field]
	if !ok {
		return "", false
	}
	if table, scoped := spec.referenceScopes[expr]; scoped {
		return table, true
	}
	table, gated := spec.filterScopes[field]
	return table, gated
}

// recordReadable says whether the caller may open one record of a table: its
// read grant, and its row scope.
func recordReadable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) (bool, error) {
	if err := requireAnalyticsRead(ctx, datasource.EntityType(table)); err != nil {
		return false, deniedOrFault(err)
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	idParam := arg(id)
	clause, err := auth.ScopeClauseFor(ctx, table, "r", arg)
	if err != nil {
		return false, err
	}
	if clause == "" {
		clause = sqlUnnarrowed
	}
	statement := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s r WHERE r.id = $%d AND (%s))",
		pgx.Identifier{table}.Sanitize(), idParam, clause)
	var readable bool
	if err := tx.QueryRow(ctx, statement, args...).Scan(&readable); err != nil {
		return false, fmt.Errorf("compose: checking a filtered record is readable: %w", err)
	}
	return readable, nil
}

// deniedOrFault turns a refusal into a plain no, and keeps every other error.
func deniedOrFault(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	return err
}
