// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A bound question card asks a saved question the report keeps whole, filters
// included. The selection every reader receives carries only the card's shape
// and an opaque reference; the question lives in the revision's
// question_bindings, which only the Questions seam reads.

// QuestionCard is one question card as the Questions seam asks it.
type QuestionCard struct {
	ID       string
	Question crmcontracts.ReportingCardQuestion
	// Bound is the kept question of a bound card. It is nil for a card that
	// asks its shape alone, and for a bound card whose question the revision
	// does not hold, which no reader may then be answered.
	Bound json.RawMessage
	// Binding is what every reader may know of the kept question.
	Binding *crmcontracts.ReportingQuestionBinding
}

// Binding is a saved question resolved for a card as its author may ask it.
type Binding struct {
	// Query is the question whole, in the engine's own encoding.
	Query json.RawMessage
	// Question is the shape every reader of the report may see.
	Question crmcontracts.ReportingCardQuestion
	Filtered bool
	Basis    *crmcontracts.ReportingPeriodBasis
}

// keptQuestion is one entry of a revision's question_bindings. The public
// half is kept beside the question so a client cannot send a shape that
// differs from what the server asks.
type keptQuestion struct {
	Query       json.RawMessage                    `json:"query"`
	Question    crmcontracts.ReportingCardQuestion `json:"question"`
	Filtered    bool                               `json:"filtered"`
	Basis       *crmcontracts.ReportingPeriodBasis `json:"period_basis,omitempty"`
	SourceRunID ids.UUID                           `json:"source_run_id"`
	BoundAt     time.Time                          `json:"bound_at"`
}

// card is the question a reader of the selection sees for this binding.
func (k keptQuestion) card(ref string) crmcontracts.ReportingCardQuestion {
	out := k.Question
	out.SavedQuestionId = nil
	out.Binding = &ref
	return out
}

// bindQuestions resolves every question card of a selection about to be
// stored: a card naming a saved question is bound to it now, and a card
// carrying a reference keeps the question this report already holds under it.
// It answers the bindings the revision must keep.
func (s *Service) bindQuestions(ctx context.Context, tx pgx.Tx, report ids.UUID, selection *crmcontracts.ReportingSelection) (map[string]keptQuestion, error) {
	out := map[string]keptQuestion{}
	if selection.Layout == nil {
		return out, nil
	}
	held, err := reportBindings(ctx, tx, report)
	if err != nil {
		return nil, err
	}
	for i := range *selection.Layout {
		cards := (*selection.Layout)[i].Cards
		for j := range cards {
			question := cards[j].Question
			if question == nil {
				continue
			}
			ref, kept, err := s.bindQuestion(ctx, tx, *question, held)
			if err != nil {
				return nil, err
			}
			if ref == "" {
				continue
			}
			out[ref] = kept
			bound := kept.card(ref)
			cards[j].Question = &bound
		}
	}
	return out, nil
}

// bindQuestion answers the binding one card keeps, or no reference for a card
// that asks its shape alone.
func (s *Service) bindQuestion(ctx context.Context, tx pgx.Tx, question crmcontracts.ReportingCardQuestion, held map[string]keptQuestion) (string, keptQuestion, error) {
	switch {
	case question.SavedQuestionId != nil:
		if s.questions == nil {
			return "", keptQuestion{}, refuse("selection.layout", "unsupported", "a saved question cannot be placed on a card here")
		}
		run := ids.UUID(*question.SavedQuestionId)
		binding, err := s.questions.BindSavedQuestion(ctx, tx, run)
		if err != nil {
			return "", keptQuestion{}, err
		}
		return ids.NewV7().String(), keptQuestion{
			Query: binding.Query, Question: binding.Question, Filtered: binding.Filtered,
			Basis: binding.Basis, SourceRunID: run, BoundAt: s.now(),
		}, nil
	case question.Binding != nil:
		kept, ok := held[*question.Binding]
		if !ok {
			return "", keptQuestion{}, refuse("selection.layout", "invalid", "place the saved question again; this report holds no question under that card's reference")
		}
		return *question.Binding, kept, nil
	}
	return "", keptQuestion{}, nil
}

// reportBindings answers the questions all revisions of the report keep, so
// a draft, a publish and a restore carry a binding forward by its reference.
func reportBindings(ctx context.Context, tx pgx.Tx, report ids.UUID) (map[string]keptQuestion, error) {
	var b bindings
	query := "SELECT question_bindings FROM report_definition_revision WHERE report_id=" + b.add(report)
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	out := map[string]keptQuestion{}
	var revision map[string]keptQuestion
	_, err = pgx.ForEachRow(rows, []any{&revision}, func() error {
		for ref, kept := range revision {
			out[ref] = kept
		}
		revision = nil
		return nil
	})
	return out, err
}

// revisionBindings is what one revision keeps.
func revisionBindings(ctx context.Context, tx pgx.Tx, report ids.UUID, revision int64) (map[string]keptQuestion, error) {
	var b bindings
	query := "SELECT question_bindings FROM report_definition_revision WHERE report_id=" + b.add(report) + " AND revision=" + b.add(revision)
	var out map[string]keptQuestion
	err := tx.QueryRow(ctx, query, b.values...).Scan(&out)
	return out, storedError(err)
}

// questionCards lists the selection's question cards with what each asks.
func questionCards(selection crmcontracts.ReportingSelection, kept map[string]keptQuestion) []QuestionCard {
	cards := QuestionCards(selection)
	out := make([]QuestionCard, 0, len(cards))
	for _, card := range cards {
		asked := QuestionCard{ID: card.Id, Question: *card.Question}
		if ref := card.Question.Binding; ref != nil {
			asked.Binding = &crmcontracts.ReportingQuestionBinding{Filtered: true}
			if question, ok := kept[*ref]; ok {
				asked.Bound = question.Query
				asked.Binding = &crmcontracts.ReportingQuestionBinding{Filtered: question.Filtered, PeriodBasis: question.Basis}
			}
		}
		out = append(out, asked)
	}
	return out
}

// QuestionCardIn is one question card of a revision the caller may read, with
// the population it is asked over: the published revision, or the draft for an
// editor, as an evaluation reads them.
func (s *Service) QuestionCardIn(ctx context.Context, tx pgx.Tx, id ids.UUID, revision *int64, cardID string) (QuestionCard, crmcontracts.ReportingScope, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return QuestionCard{}, crmcontracts.ReportingScope{}, err
	}
	report, err := s.report(ctx, tx, id, false)
	if err != nil {
		return QuestionCard{}, crmcontracts.ReportingScope{}, err
	}
	chosen := report.Revision
	if revision != nil {
		chosen = *revision
	}
	selection, _, err := s.evaluableRevision(ctx, tx, report, chosen)
	if err != nil {
		return QuestionCard{}, crmcontracts.ReportingScope{}, err
	}
	kept, err := revisionBindings(ctx, tx, id, chosen)
	if err != nil {
		return QuestionCard{}, crmcontracts.ReportingScope{}, err
	}
	for _, card := range questionCards(selection, kept) {
		if card.ID == cardID {
			return card, selection.Scope, nil
		}
	}
	return QuestionCard{}, crmcontracts.ReportingScope{}, apperrors.ErrNotFound
}
