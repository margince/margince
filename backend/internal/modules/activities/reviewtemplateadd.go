// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// reviewTemplateObject names the table in the audit trail and in the
// reporting.changed event a template write announces.
const reviewTemplateObject = "activity_review_template"

// reviewTemplateKeyShape is the table's own key CHECK, so a refusal names the
// field before the insert fails on it.
var reviewTemplateKeyShape = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

const reviewTemplateKeyMax = 63

const (
	reviewKeyField     = "key"
	reviewOutcomeField = "outcome"
)

// CreateReviewTemplate adds the template an outcome asks from now on. A
// closing asks one review, so the template asked before for that outcome is
// retired in the same transaction.
func (s *Store) CreateReviewTemplate(ctx context.Context, req crmcontracts.CreateActivityReviewTemplateRequest) (crmcontracts.ActivityReviewTemplate, error) {
	if err := auth.Require(ctx, "custom_field", principal.ActionCreate); err != nil {
		return crmcontracts.ActivityReviewTemplate{}, err
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		return crmcontracts.ActivityReviewTemplate{}, &ReviewDefinitionError{Field: "label", Detail: "A template needs a name"}
	}
	outcome := string(req.Outcome)
	if outcome != OutcomeWon && outcome != OutcomeLost {
		return crmcontracts.ActivityReviewTemplate{}, &ReviewDefinitionError{Field: reviewOutcomeField, Detail: "A template is for a won or a lost deal"}
	}
	questions := reviewQuestionsFromWire(req.Questions)
	if err := validateReviewQuestions(questions); err != nil {
		return crmcontracts.ActivityReviewTemplate{}, err
	}
	if req.Key != nil && !reviewTemplateKeyShape.MatchString(*req.Key) {
		return crmcontracts.ActivityReviewTemplate{}, &ReviewDefinitionError{Field: reviewKeyField, Detail: "A key starts with a letter and holds only lowercase letters, digits and underscores"}
	}
	raw, err := json.Marshal(questions)
	if err != nil {
		return crmcontracts.ActivityReviewTemplate{}, fmt.Errorf("encode template: %w", err)
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.ActivityReviewTemplate{}, err
	}
	var result crmcontracts.ActivityReviewTemplate
	err = s.tx(ctx, func(tx pgx.Tx) error {
		key := ""
		if req.Key != nil {
			key = *req.Key
		} else if key, err = freeReviewTemplateKey(ctx, tx, derivedReviewTemplateKey(label)); err != nil {
			return err
		}
		if err := retireAskedTemplate(ctx, tx, outcome); err != nil {
			return err
		}
		result, err = scanReviewTemplate(tx.QueryRow(ctx,
			`INSERT INTO activity_review_template (key, label, outcome, questions, source, captured_by)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+reviewTemplateColumns,
			key, label, outcome, raw, provenance.RecordSourceManual, by))
		if storekit.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert template: %w", err)
		}
		return recordReviewTemplateCreate(ctx, tx, ids.UUID(result.Id), map[string]any{
			reviewKeyField: key, "label": label, reviewOutcomeField: outcome, reviewQuestionsField: result.Questions,
		})
	})
	return result, err
}

// retireAskedTemplate retires the template an outcome asks now, so the one
// being added or restored becomes the only active one. The partial unique
// index on outcome refuses a second active template if this is skipped.
func retireAskedTemplate(ctx context.Context, tx pgx.Tx, outcome string) error {
	rows, err := tx.Query(ctx,
		`UPDATE activity_review_template SET active = false
		  WHERE outcome = $1 AND active RETURNING id`, outcome)
	if err != nil {
		return fmt.Errorf("retire the asked template: %w", err)
	}
	retired, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("retire the asked template: %w", err)
	}
	for _, id := range retired {
		if err := recordReviewTemplateUpdate(ctx, tx, id,
			map[string]any{reviewActiveField: true}, map[string]any{reviewActiveField: false}); err != nil {
			return err
		}
	}
	return nil
}

// recordReviewTemplateCreate and recordReviewTemplateUpdate are the audit and
// event half of every template write. reporting.changed tells a reader of the
// template list to read it again.
func recordReviewTemplateCreate(ctx context.Context, tx pgx.Tx, id ids.UUID, after map[string]any) error {
	auditID, err := storekit.Audit(ctx, tx, "create", reviewTemplateObject, id, nil, after)
	if err != nil {
		return err
	}
	return announceReviewTemplateChange(ctx, tx, auditID, "create", id)
}

func recordReviewTemplateUpdate(ctx context.Context, tx pgx.Tx, id ids.UUID, before, after map[string]any) error {
	auditID, err := storekit.Audit(ctx, tx, "update", reviewTemplateObject, id, before, after)
	if err != nil {
		return err
	}
	return announceReviewTemplateChange(ctx, tx, auditID, "update", id)
}

func announceReviewTemplateChange(ctx context.Context, tx pgx.Tx, auditID ids.UUID, action string, id ids.UUID) error {
	return storekit.Emit(ctx, tx, auditID, "reporting.changed", reviewTemplateObject, id,
		crmcontracts.InternalEventReportingChanged{Object: reviewTemplateObject, Action: action})
}

// derivedReviewTemplateKey turns the label into a key the table's CHECK
// accepts. A label with no Latin letters, or one that opens with a digit,
// still gets a key.
func derivedReviewTemplateKey(label string) string {
	key := cutReviewTemplateKey(values.CatalogKey(label))
	if !reviewTemplateKeyShape.MatchString(key) {
		key = cutReviewTemplateKey("review_" + key)
	}
	return key
}

func cutReviewTemplateKey(key string) string {
	return strings.TrimRight(key[:min(len(key), reviewTemplateKeyMax)], "_")
}

// freeReviewTemplateKey answers base, or base with the first free number when
// a template already holds it. A key is never shown to the admin, so a
// second "Win review" gets win_review_2 rather than a refusal.
func freeReviewTemplateKey(ctx context.Context, tx pgx.Tx, base string) (string, error) {
	rows, err := tx.Query(ctx,
		`SELECT key FROM activity_review_template WHERE starts_with(key, $1)`, base)
	if err != nil {
		return "", fmt.Errorf("read template keys: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("read template keys: %w", err)
	}
	taken := make(map[string]bool, len(held))
	for _, key := range held {
		taken[key] = true
	}
	key := base
	for n := 2; taken[key]; n++ {
		suffix := "_" + strconv.Itoa(n)
		key = base[:min(len(base), reviewTemplateKeyMax-len(suffix))] + suffix
	}
	return key, nil
}
