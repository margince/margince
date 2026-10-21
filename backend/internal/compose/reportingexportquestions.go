// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"slices"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
)

// questionCSVRows writes each question card of an edition in page order: one
// row per kept answer row and measure, or one row saying why there is none. A
// question's text reaches the file only through formulacell.Guard, since a
// measure's alias and a group's name are typed by people.
func questionCSVRows(edition crmcontracts.ReportingEdition) [][]string {
	kept := map[string]crmcontracts.ReportingEditionQuestion{}
	if edition.Questions != nil {
		for _, question := range *edition.Questions {
			kept[question.CardId] = question
		}
	}
	captured := edition.Evaluation.Context.EvaluatedAt.Format(time.RFC3339Nano)
	rows := [][]string{}
	for _, card := range reporting.QuestionCards(edition.Evaluation.Selection) {
		cell := func(label, value, context, coverage string) []string {
			return []string{formulacell.Guard(card.Question.Entity), "question:" + card.Id, formulacell.Guard(label), value, "", "", "", "", "", formulacell.Guard(context), coverage, captured}
		}
		question, found := kept[card.Id]
		switch {
		case !found:
			rows = append(rows, cell("", "", "", "not_kept"))
		case question.Status != crmcontracts.ReportingEditionQuestionStatusKept || question.Answer == nil:
			rows = append(rows, cell("", "", "", questionCoverage(question)))
		default:
			rows = append(rows, keptAnswerRows(*card.Question, *question.Answer, cell)...)
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

func keptAnswerRows(question crmcontracts.ReportingCardQuestion, answer crmcontracts.AnalyticsAnswer, cell func(label, value, context, coverage string) []string) [][]string {
	groupBy := []string{}
	if question.GroupBy != nil {
		groupBy = *question.GroupBy
	}
	rows := [][]string{}
	for _, row := range answer.Rows {
		if row[withheldColumn] == true {
			rows = append(rows, cell("", "", "", "withheld"))
			continue
		}
		label := groupLabel(groupBy, row, answer.Labels)
		for _, column := range answer.Columns {
			if slices.Contains(groupBy, column) {
				continue
			}
			rows = append(rows, cell(label, csvAnswerNumber(row[column]), column, "ok"))
		}
	}
	return rows
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
