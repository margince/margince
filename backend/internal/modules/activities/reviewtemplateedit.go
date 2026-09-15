// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	reviewQuestionsField = "questions"
	reviewMultipleChoice = "multiselect"
)

// ReviewDefinitionError identifies an invalid question or choice without
// accepting a partial template that would lose an administrator's intent.
type ReviewDefinitionError struct{ Field, Detail string }

func (e *ReviewDefinitionError) Error() string { return e.Detail }

// FieldFault connects definition errors to the standard validation response.
func (e *ReviewDefinitionError) FieldFault() (string, string, string) {
	return e.Field, "invalid_review_definition", e.Detail
}

func validateReviewQuestions(questions []ReviewQuestion) error {
	if len(questions) == 0 {
		return &ReviewDefinitionError{Field: reviewQuestionsField, Detail: "At least one question is required"}
	}
	seen := map[string]bool{}
	for _, q := range questions {
		if strings.TrimSpace(q.Key) == "" || strings.TrimSpace(q.Label) == "" || seen[q.Key] {
			return &ReviewDefinitionError{Field: reviewQuestionsField, Detail: "Questions need unique keys and non-empty labels"}
		}
		seen[q.Key] = true
		switch q.Type {
		case "text":
			if len(q.Options) != 0 {
				return &ReviewDefinitionError{Field: reviewQuestionsField, Detail: "Text questions cannot carry options"}
			}
		case reviewMultipleChoice:
			if len(q.Options) == 0 {
				return &ReviewDefinitionError{Field: reviewQuestionsField, Detail: "Multiple-choice questions need options"}
			}
			options := map[string]bool{}
			for _, option := range q.Options {
				if strings.TrimSpace(option) == "" || options[option] {
					return &ReviewDefinitionError{Field: reviewQuestionsField, Detail: "Options must be non-empty and unique"}
				}
				options[option] = true
			}
		default:
			return &ReviewDefinitionError{Field: reviewQuestionsField, Detail: "Unsupported question type"}
		}
	}
	return nil
}

func validateReviewAnswers(questions []ReviewQuestion, answers map[string]string, choices map[string][]string) error {
	var textQuestions []ReviewQuestion
	asked := map[string]ReviewQuestion{}
	for _, q := range questions {
		asked[q.Key] = q
		if q.Type == "text" {
			textQuestions = append(textQuestions, q)
		}
	}
	if err := validateAnswers(textQuestions, answers); err != nil {
		return err
	}
	for key, selected := range choices {
		q, ok := asked[key]
		if !ok || q.Type != reviewMultipleChoice {
			return &UnknownAnswerError{Key: key}
		}
		allowed := map[string]bool{}
		for _, option := range q.Options {
			allowed[option] = true
		}
		seen := map[string]bool{}
		for _, value := range selected {
			if !allowed[value] || seen[value] {
				return &ReviewDefinitionError{Field: "choice_answers", Detail: "Selected choices must be unique options from the question"}
			}
			seen[value] = true
		}
	}
	for _, q := range questions {
		if q.Type == reviewMultipleChoice && q.Required && len(choices[q.Key]) == 0 {
			return &MissingAnswerError{Key: q.Key, Label: q.Label}
		}
	}
	return nil
}

// UpdateReviewTemplate changes only future questions. Submitted responses keep
// their own snapshot; version checking prevents overwriting a concurrent edit.
func (s *Store) UpdateReviewTemplate(ctx context.Context, id ids.UUID, req crmcontracts.UpdateActivityReviewTemplateRequest) (crmcontracts.ActivityReviewTemplate, error) {
	if err := auth.Require(ctx, "custom_field", principal.ActionUpdate); err != nil {
		return crmcontracts.ActivityReviewTemplate{}, err
	}
	questions := make([]ReviewQuestion, len(req.Questions))
	for i, q := range req.Questions {
		questions[i] = ReviewQuestion{Key: q.Key, Label: q.Label, Type: string(q.Type), Required: q.Required}
		if q.Options != nil {
			questions[i].Options = *q.Options
		}
	}
	if err := validateReviewQuestions(questions); err != nil {
		return crmcontracts.ActivityReviewTemplate{}, err
	}
	raw, err := json.Marshal(questions)
	if err != nil {
		return crmcontracts.ActivityReviewTemplate{}, fmt.Errorf("encode template: %w", err)
	}
	var result crmcontracts.ActivityReviewTemplate
	err = s.tx(ctx, func(tx pgx.Tx) error {
		before, err := scanReviewTemplate(tx.QueryRow(ctx, `SELECT id, key, label, outcome, questions, version, active, system, created_at, updated_at FROM activity_review_template WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read template: %w", err)
		}
		if before.Version == nil || *before.Version != req.Version {
			return apperrors.ErrConflict
		}
		result, err = scanReviewTemplate(tx.QueryRow(ctx, `UPDATE activity_review_template SET questions=$2 WHERE id=$1 RETURNING id, key, label, outcome, questions, version, active, system, created_at, updated_at`, id, raw))
		if err != nil {
			return fmt.Errorf("update template: %w", err)
		}
		// Audit-only catalog configuration, like custom_field. A template is not a
		// record activity and has no event in the closed event catalog.
		_, err = storekit.AuditWithEvidence(ctx, tx, "update", "activity_review_template", id,
			map[string]any{reviewQuestionsField: before.Questions}, map[string]any{reviewQuestionsField: result.Questions}, nil)
		return err
	})
	return result, err
}
