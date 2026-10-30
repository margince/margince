// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Questions binds a card to a saved question, answers a report's question
// cards for a sent edition, and later judges what of a kept answer one reader
// may see. A seam because the analytics engine lives in compose.
type Questions interface {
	// BindSavedQuestion reads a saved question whole as the caller may ask it now.
	BindSavedQuestion(ctx context.Context, tx pgx.Tx, savedQuestion ids.UUID) (Binding, error)
	// Capture asks the card's question as the edition's sender.
	Capture(ctx context.Context, tx pgx.Tx, card QuestionCard, scope crmcontracts.ReportingScope) (CapturedQuestion, error)
	// Reveal answers ok=false when the reader may not ask the question now.
	// Otherwise it returns the kept answer with every row withheld whose
	// records the reader may not all read.
	Reveal(ctx context.Context, tx pgx.Tx, card QuestionCard, scope crmcontracts.ReportingScope, kept KeptQuestion) (answer crmcontracts.AnalyticsAnswer, ok bool, err error)
}

// CapturedQuestion is one question card's answer at capture.
type CapturedQuestion struct {
	Entity string
	// Reason is set when nothing is kept, and the answer is then empty.
	Reason crmcontracts.ReportingEditionQuestionReason
	KeptQuestion
}

// KeptQuestion is the answer as its sender was given it. Sources[i] are the
// records behind Answer.Rows[i]; a row the floor withheld has none.
type KeptQuestion struct {
	Answer  crmcontracts.AnalyticsAnswer
	Sources [][]QuestionSource
}

// QuestionSource is one record an answer row was computed from.
type QuestionSource struct {
	Type string
	ID   ids.UUID
}

// WithQuestions lets sent editions keep their question cards' answers. Without
// it an edition keeps none, and every question card reads as not kept.
func (s *Service) WithQuestions(questions Questions) *Service {
	s.questions = questions
	return s
}

// QuestionCards lists the layout's question cards in page order.
func QuestionCards(selection crmcontracts.ReportingSelection) []crmcontracts.ReportingCard {
	cards := []crmcontracts.ReportingCard{}
	if selection.Layout == nil {
		return cards
	}
	for _, section := range *selection.Layout {
		for _, card := range section.Cards {
			if card.Kind == questionKind && card.Question != nil {
				cards = append(cards, card)
			}
		}
	}
	return cards
}

// captureQuestions answers every question card of a revision as the sender.
// An edition keeps an answer only under a retention policy, which also erases
// it.
func (s *Service) captureQuestions(ctx context.Context, tx pgx.Tx, report ids.UUID, revision int64, selection crmcontracts.ReportingSelection) (map[string]CapturedQuestion, error) {
	if len(QuestionCards(selection)) == 0 || s.questions == nil {
		return map[string]CapturedQuestion{}, nil
	}
	kept, err := revisionBindings(ctx, tx, report, revision)
	if err != nil {
		return nil, err
	}
	cards := questionCards(selection, kept)
	out := make(map[string]CapturedQuestion, len(cards))
	ready, err := scheduleReady(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, card := range cards {
		if !ready {
			out[card.ID] = CapturedQuestion{Entity: card.Question.Entity, Reason: crmcontracts.ReportingEditionQuestionReasonRetentionUnconfigured}
			continue
		}
		captured, err := s.questions.Capture(ctx, tx, card, selection.Scope)
		if err != nil {
			return nil, err
		}
		out[card.ID] = captured
	}
	return out, nil
}

func writeEditionQuestions(ctx context.Context, tx pgx.Tx, edition openapi_types.UUID, captured map[string]CapturedQuestion) error {
	for cardID, question := range captured {
		var answer, reason any
		if question.Reason != "" {
			reason = question.Reason
		} else {
			raw, err := encode(question.Answer)
			if err != nil {
				return err
			}
			answer = raw
		}
		var b bindings
		values := []string{b.add(edition), b.add(cardID), b.add(question.Entity), b.add(answer), b.add(reason)}
		if _, err := tx.Exec(ctx, "INSERT INTO report_edition_question(edition_id,card_id,entity,answer,reason) VALUES ("+strings.Join(values, ",")+")", b.values...); err != nil {
			return err
		}
		if err := writeQuestionSources(ctx, tx, edition, cardID, question.Sources); err != nil {
			return err
		}
	}
	return nil
}

func writeQuestionSources(ctx context.Context, tx pgx.Tx, edition openapi_types.UUID, cardID string, sources [][]QuestionSource) error {
	type sourceRow struct {
		row    int
		source QuestionSource
	}
	flat := []sourceRow{}
	for row, records := range sources {
		for _, source := range records {
			flat = append(flat, sourceRow{row, source})
		}
	}
	// Bound the parameter count per round trip, as edition contributions do.
	for start := 0; start < len(flat); start += 250 {
		var b bindings
		tuples := []string{}
		for _, item := range flat[start:min(start+250, len(flat))] {
			values := []string{b.add(edition), b.add(cardID), b.add(item.row), b.add(item.source.Type), b.add(item.source.ID)}
			tuples = append(tuples, "("+strings.Join(values, ",")+")")
		}
		if _, err := tx.Exec(ctx, "INSERT INTO report_edition_question_source(edition_id,card_id,row_index,source_type,source_id) VALUES "+strings.Join(tuples, ",")+" ON CONFLICT DO NOTHING", b.values...); err != nil {
			return err
		}
	}
	return nil
}

type storedQuestion struct {
	cardID string
	answer []byte
	reason *string
}

// editionQuestions reads what the edition kept for each question card, as this
// reader may see it.
func (s *Service) editionQuestions(ctx context.Context, tx pgx.Tx, edition *crmcontracts.ReportingEdition) error {
	var b bindings
	query := "SELECT card_id,answer,reason FROM report_edition_question WHERE edition_id=" + b.add(edition.Id) + " ORDER BY card_id"
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return err
	}
	stored, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedQuestion, error) {
		var q storedQuestion
		return q, row.Scan(&q.cardID, &q.answer, &q.reason)
	})
	if err != nil || len(stored) == 0 {
		return err
	}
	kept, err := revisionBindings(ctx, tx, ids.UUID(edition.ReportId), edition.ReportRevision)
	if err != nil {
		return err
	}
	cards := map[string]QuestionCard{}
	for _, card := range questionCards(edition.Evaluation.Selection, kept) {
		cards[card.ID] = card
	}
	out := []crmcontracts.ReportingEditionQuestion{}
	for _, q := range stored {
		card, placed := cards[q.cardID]
		if !placed {
			continue
		}
		judged, err := s.revealQuestion(ctx, tx, edition, card, q)
		if err != nil {
			return err
		}
		out = append(out, judged)
	}
	edition.Questions = &out
	return nil
}

func (s *Service) revealQuestion(ctx context.Context, tx pgx.Tx, edition *crmcontracts.ReportingEdition, card QuestionCard, q storedQuestion) (crmcontracts.ReportingEditionQuestion, error) {
	out := crmcontracts.ReportingEditionQuestion{CardId: q.cardID, Status: crmcontracts.ReportingEditionQuestionStatusWithheld}
	if q.reason != nil {
		reason := crmcontracts.ReportingEditionQuestionReason(*q.reason)
		out.Status, out.Reason = crmcontracts.ReportingEditionQuestionStatusNotKept, &reason
		return out, nil
	}
	if s.questions == nil {
		return out, nil
	}
	var kept KeptQuestion
	if err := json.Unmarshal(q.answer, &kept.Answer); err != nil {
		return out, err
	}
	sources, err := questionSources(ctx, tx, edition.Id, q.cardID, len(kept.Answer.Rows))
	if err != nil {
		return out, err
	}
	kept.Sources = sources
	answer, ok, err := s.questions.Reveal(ctx, tx, card, edition.Evaluation.Selection.Scope, kept)
	if err != nil || !ok {
		return out, err
	}
	out.Status, out.Answer = crmcontracts.ReportingEditionQuestionStatusKept, &answer
	return out, nil
}

func questionSources(ctx context.Context, tx pgx.Tx, edition openapi_types.UUID, cardID string, rowCount int) ([][]QuestionSource, error) {
	var b bindings
	query := "SELECT row_index,source_type,source_id FROM report_edition_question_source WHERE edition_id=" + b.add(edition) + " AND card_id=" + b.add(cardID)
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	out := make([][]QuestionSource, rowCount)
	var index int
	var source QuestionSource
	_, err = pgx.ForEachRow(rows, []any{&index, &source.Type, &source.ID}, func() error {
		if index >= 0 && index < rowCount {
			out[index] = append(out[index], source)
		}
		return nil
	})
	return out, err
}
