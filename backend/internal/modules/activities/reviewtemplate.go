// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The questions an outcome review asks.
//
// A template is EDITABLE, and that is the whole reason a response freezes the
// questions it was asked rather than pointing back here. Rewording a question
// must not change what a review written last quarter appears to have asked, and
// retiring a template must not make old reviews unreadable.
//
// So this file only ever answers "what should we ask NOW". What was asked then
// lives on the response.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReviewQuestion is one question as the template states it and as a response
// freezes it. The two are the same shape on purpose: freezing is a copy, not a
// translation, so a reader comparing an old review against the current template
// is comparing like with like.
type ReviewQuestion struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// ReviewTemplate is one set of questions, for one outcome.
type ReviewTemplate struct {
	ID        string
	Key       string
	Label     string
	Outcome   string
	Questions []ReviewQuestion
	Version   int64
	Active    bool
	System    bool
}

// The two outcomes a review can be about. They are the deal's own terminal
// semantics, not a second vocabulary: a review is about what the deal DID.
const (
	OutcomeWon  = "won"
	OutcomeLost = "lost"
)

// ReviewTemplateFor answers the live template for one outcome, or reports that
// the installation has retired it.
//
// Keyed on the outcome rather than chosen by the caller, because offering a win
// review and a loss review at the same close is asking somebody to do filing
// the record can do itself. An installation that retires both has turned the
// feature off, which is a real choice and reads as "no review to write" rather
// than as an error.
func ReviewTemplateFor(ctx context.Context, tx pgx.Tx, outcome string) (ReviewTemplate, bool, error) {
	var t ReviewTemplate
	var questions []byte
	err := tx.QueryRow(ctx,
		`SELECT id, key, label, outcome, questions, version, active, system
		   FROM activity_review_template
		  WHERE outcome = $1 AND active AND archived_at IS NULL
		  ORDER BY created_at
		  LIMIT 1`, outcome).
		Scan(&t.ID, &t.Key, &t.Label, &t.Outcome, &questions, &t.Version, &t.Active, &t.System)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewTemplate{}, false, nil
	}
	if err != nil {
		return ReviewTemplate{}, false, fmt.Errorf("read the review template: %w", err)
	}
	if err := json.Unmarshal(questions, &t.Questions); err != nil {
		return ReviewTemplate{}, false, fmt.Errorf("the review template's questions are not readable: %w", err)
	}
	return t, true, nil
}

// ListReviewTemplates answers every template an administrator can see,
// including retired ones: retiring keeps history, and an admin screen that
// hid retired rows would make an old review's template look deleted.
func (s *Store) ListReviewTemplates(ctx context.Context) ([]crmcontracts.ActivityReviewTemplate, error) {
	if err := auth.Require(ctx, "custom_field", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.ActivityReviewTemplate
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, key, label, outcome, questions, version, active, system, created_at, updated_at
			   FROM activity_review_template
			  WHERE archived_at IS NULL
			  ORDER BY outcome, created_at`)
		if err != nil {
			return fmt.Errorf("list review templates: %w", err)
		}
		defer rows.Close()
		out = []crmcontracts.ActivityReviewTemplate{}
		for rows.Next() {
			wire, err := scanReviewTemplate(rows)
			if err != nil {
				return err
			}
			out = append(out, wire)
		}
		return rows.Err()
	})
	return out, err
}

// scanReviewTemplate reads one row into the wire shape, decoding the questions
// so a client never has to parse a JSON string out of a JSON field.
func scanReviewTemplate(row pgx.Row) (crmcontracts.ActivityReviewTemplate, error) {
	var wire crmcontracts.ActivityReviewTemplate
	var id ids.UUID
	var questions []byte
	var outcome string
	if err := row.Scan(&id, &wire.Key, &wire.Label, &outcome, &questions,
		&wire.Version, &wire.Active, &wire.System, &wire.CreatedAt, &wire.UpdatedAt); err != nil {
		return crmcontracts.ActivityReviewTemplate{}, err
	}
	var decoded []ReviewQuestion
	if err := json.Unmarshal(questions, &decoded); err != nil {
		return crmcontracts.ActivityReviewTemplate{}, fmt.Errorf("template questions are not readable: %w", err)
	}
	wire.Id = openapi_types.UUID(id)
	wire.Outcome = crmcontracts.ActivityReviewTemplateOutcome(outcome)
	wire.Questions = wireQuestions(decoded)
	return wire, nil
}

// wireQuestions turns the stored questions into the contract's own shape.
func wireQuestions(questions []ReviewQuestion) []crmcontracts.ReviewQuestion {
	out := make([]crmcontracts.ReviewQuestion, 0, len(questions))
	for _, q := range questions {
		out = append(out, crmcontracts.ReviewQuestion{
			Key: q.Key, Label: q.Label,
			Type:     crmcontracts.ReviewQuestionType(q.Type),
			Required: q.Required,
		})
	}
	return out
}

// UnknownAnswerError refuses an answer to a question the template never asked.
// Storing it would put a key in the frozen answers that the frozen questions
// cannot explain, and every later reader would have to guess what it meant.
type UnknownAnswerError struct{ Key string }

func (e *UnknownAnswerError) Error() string {
	return "this review has no question called " + e.Key
}

// FieldFault names the offending answer key.
func (e *UnknownAnswerError) FieldFault() (field, code, message string) {
	return "answers", "unknown_question", e.Error()
}

// MissingAnswerError refuses a submission that left a required question blank.
type MissingAnswerError struct{ Key, Label string }

func (e *MissingAnswerError) Error() string {
	return "this review needs an answer to: " + e.Label
}

// FieldFault names the question still owed.
func (e *MissingAnswerError) FieldFault() (field, code, message string) {
	return "answers", "answer_required", e.Error()
}

// validateAnswers checks a submission against the questions it will be frozen
// beside, so the stored pair can never disagree: every answer belongs to a
// question, and every required question has one.
//
// Done here rather than by a CHECK because the rule is about two JSON documents
// agreeing with each other, and a constraint that tried to say it would be a
// rule nobody could read.
func validateAnswers(questions []ReviewQuestion, answers map[string]string) error {
	asked := make(map[string]ReviewQuestion, len(questions))
	for _, q := range questions {
		asked[q.Key] = q
	}
	for key := range answers {
		if _, ok := asked[key]; !ok {
			return &UnknownAnswerError{Key: key}
		}
	}
	for _, q := range questions {
		if !q.Required {
			continue
		}
		if strings.TrimSpace(answers[q.Key]) == "" {
			return &MissingAnswerError{Key: q.Key, Label: q.Label}
		}
	}
	return nil
}

// ensureReviewAuthority is the object permission a review write needs. Row
// scope is the DEAL's, checked by the caller that holds the deal.
func ensureReviewAuthority(ctx context.Context) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	return nil
}

// errReviewTemplateRetired reports that the installation has no live template
// for this outcome, which is a configuration answer rather than a fault.
var errReviewTemplateRetired = fmt.Errorf("no live review template for this outcome: %w", apperrors.ErrNotFound)
