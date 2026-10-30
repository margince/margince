// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Scope is a population as a report or a question names it. An empty Kind
// names none.
type Scope struct {
	Kind string
	ID   string
}

// scopeWorkspace is the population every other one narrows.
const scopeWorkspace = "workspace"

// Narrow asks a bound question inside a report: the report's population and
// the question's own, and every filter the question carries. Both must hold,
// so inputs that cannot hold at once are refused with the reason. Nothing is
// dropped to make the question run, since a broader answer would be one
// nobody asked for.
func Narrow(entity Entity, report Scope, q Query) (Query, error) {
	scope, err := intersectScope(report, Scope{Kind: q.ScopeKind, ID: q.ScopeID})
	if err != nil {
		return Query{}, err
	}
	q.ScopeKind, q.ScopeID = scope.Kind, scope.ID
	if err := refuseContradictions(entity, q.Filters); err != nil {
		return Query{}, err
	}
	return q, nil
}

// intersectScope keeps the narrower of two populations when one holds the
// other. A report naming none asks over the reader's own default. That holds
// whatever they may measure, so the question's population stands, and the
// engine judges it for them.
func intersectScope(report, question Scope) (Scope, error) {
	switch {
	case question.Kind == "":
		return report, nil
	case report.Kind == "" || report.Kind == scopeWorkspace || report == question:
		return question, nil
	case question.Kind == scopeWorkspace:
		return report, nil
	}
	return Scope{}, &RefusalError{
		Kind:    RefusalInvalid,
		Message: "this question measures a different team or person than the report does",
		Suggest: "pick a question saved for the whole company or for the report's own team or person",
	}
}

// refuseContradictions refuses filters no record can meet at once: one field
// asked to equal two values, or kept periods that do not overlap. Neither
// value is quoted, because the refusal must say no more than the question
// does.
func refuseContradictions(entity Entity, filters []Filter) error {
	equal := map[string]string{}
	var from, to time.Time
	for _, f := range filters {
		start, end, kept, err := keptSpan(entity, f)
		if err != nil {
			return err
		}
		if f.Op != OpEq {
			continue
		}
		value := fmt.Sprint(f.Value)
		if seen, ok := equal[f.Field]; ok && seen != value {
			return contradiction(fmt.Sprintf("the question asks for two values of %s at once", httperr.QuoteCaller(f.Field)))
		}
		equal[f.Field] = value
		if !kept {
			continue
		}
		if from.IsZero() || start.After(from) {
			from = start
		}
		if to.IsZero() || end.Before(to) {
			to = end
		}
		if !from.Before(to) {
			return contradiction("the question's periods do not overlap")
		}
	}
	return nil
}

// keptSpan is the interval a period filter that keeps its basis names, and
// kept=false for any other filter.
func keptSpan(entity Entity, f Filter) (from, to time.Time, kept bool, err error) {
	field, ok := entity.Lookup(f.Field)
	if !ok || field.Period == nil || f.Basis == nil || valuelessOps[f.Op] {
		return from, to, false, nil
	}
	label, isText := f.Value.(string)
	if !isText {
		return from, to, false, unresolvedPeriod(fmt.Sprint(f.Value))
	}
	from, to, err = PeriodSpan(field.Period.Grain, label, *f.Basis)
	return from, to, err == nil, err
}

func contradiction(message string) error {
	return &RefusalError{
		Kind:    RefusalInvalid,
		Message: message,
		Suggest: "save the question again with filters that can all hold",
	}
}
