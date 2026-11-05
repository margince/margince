// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
)

// questionCSVRows writes each question card of an edition in page order. It
// writes one row per kept answer row and measure, or one saying why not. A
// question's text reaches the file only through formulacell.Guard, because
// a measure's alias and a group's name are typed by hand.
func questionCSVRows(edition crmcontracts.ReportingEdition) [][]string {
	// Retention and erasure remove every answer and leave no list. That is not
	// the same as an answer never kept, so such an edition writes no question rows.
	if edition.Questions == nil {
		return nil
	}
	kept := map[string]crmcontracts.ReportingEditionQuestion{}
	for _, question := range *edition.Questions {
		kept[question.CardId] = question
	}
	captured := edition.Evaluation.Context.EvaluatedAt.Format(time.RFC3339Nano)
	rows := [][]string{}
	for _, card := range reporting.QuestionCards(edition.Evaluation.Selection) {
		cell := func(c questionCell) []string {
			return []string{formulacell.Guard(card.Question.Entity), "question:" + card.Id, formulacell.Guard(c.label), c.value, "", "", "", c.unit, c.currency, formulacell.Guard(c.context), c.coverage, captured}
		}
		question, found := kept[card.Id]
		switch {
		case !found:
			rows = append(rows, cell(questionCell{coverage: "not_kept"}))
		case question.Status != crmcontracts.ReportingEditionQuestionStatusKept || question.Answer == nil:
			rows = append(rows, cell(questionCell{coverage: questionCoverage(question)}))
		default:
			money := answerMoney{spec: prebuiltReports[card.Question.Entity], base: edition.Evaluation.Context.Currency}
			rows = append(rows, keptAnswerRows(*card.Question, *question.Answer, money, cell)...)
		}
	}
	return rows
}

func questionCoverage(question crmcontracts.ReportingEditionQuestion) string {
	if question.Reason != nil {
		return "not_kept:" + string(*question.Reason)
	}
	return string(question.Status)
}

// questionCell is one row of a question card in the file.
type questionCell struct {
	label, value, unit, currency, context, coverage string
}

func keptAnswerRows(question crmcontracts.ReportingCardQuestion, answer crmcontracts.AnalyticsAnswer, money answerMoney, cell func(questionCell) []string) [][]string {
	asked := queryFromWire(crmcontracts.AnalyticsQuery{Entity: question.Entity, GroupBy: question.GroupBy, Measures: question.Measures})
	measures := map[string]analyticsquery.Measure{}
	for _, measure := range asked.Measures {
		measures[measure.Column()] = measure
	}
	rows := [][]string{}
	for _, row := range answer.Rows {
		if row[withheldColumn] == true {
			rows = append(rows, cell(questionCell{coverage: "withheld"}))
			continue
		}
		label := groupLabel(asked.GroupBy, row, answer.Labels)
		for _, column := range answer.Columns {
			measure, isMeasure := measures[column]
			if !isMeasure {
				continue
			}
			unit, currency, coverage := money.of(measure, row)
			value := csvAnswerNumber(row[column])
			if coverage != "ok" {
				value = ""
			}
			rows = append(rows, cell(questionCell{label: label, value: value, unit: unit, currency: currency, context: column, coverage: coverage}))
		}
	}
	return rows
}

// answerMoney reads a money measure the way the screen does. A converted
// amount is in the base currency. A native one is in its row's own currency.
type answerMoney struct {
	spec reportSpec
	base string
}

// of gives a measure's unit and currency, or a coverage saying why no single
// currency applies. A count of a money field is a count, not money.
func (m answerMoney) of(measure analyticsquery.Measure, row map[string]any) (unit, currency, coverage string) {
	if !strings.HasSuffix(measure.Field, "_minor") || measure.Fn == analyticsquery.CountAll || measure.Fn == analyticsquery.CountDistinct {
		return "", "", "ok"
	}
	if m.spec.nativeMeasures[measure.Field] {
		own, _ := row[fieldCurrency].(string)
		if own == "" {
			return "", "", "mixed_currency"
		}
		return reportingMinorUnits, own, "ok"
	}
	if m.base == "" {
		return "", "", "no_base_currency"
	}
	return reportingMinorUnits, m.base, "ok"
}

// groupLabel names a row by its group keys, a company or project by the name
// the reader was given for it.
func groupLabel(groupBy []string, row map[string]any, labels *crmcontracts.AnalyticsIdLabels) string {
	parts := make([]string, len(groupBy))
	for i, name := range groupBy {
		if row[name] == nil {
			continue
		}
		parts[i] = fmt.Sprint(row[name])
		if labels != nil {
			if named, ok := (*labels)[name][parts[i]]; ok {
				parts[i] = named
			}
		}
	}
	return strings.Join(parts, " · ")
}

// csvAnswerNumber writes a measure as the analytics answer carried it.
//
//craft:ignore naked-any an answer row's values are schemaless JSON, decoded as any
func csvAnswerNumber(value any) string {
	switch number := value.(type) {
	case nil:
		return ""
	case float64:
		return reportingCSVNumber(&number)
	default:
		return formulacell.Guard(fmt.Sprint(number))
	}
}
