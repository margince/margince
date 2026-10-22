// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const paramOperator = "operator"

// SealReviewTx binds nondeterministic predictions to their reader until activation.
func SealReviewTx(ctx context.Context, tx pgx.Tx, taskID string, review ReviewRequest, now time.Time) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return "", apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return "", err
	}
	if !validSealedReview(taskID, review, now) {
		return "", apperrors.ErrInvalidArgument
	}
	if err := purgeExpiredReviews(ctx, tx, now); err != nil {
		return "", err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	hash := sha256.Sum256([]byte(token))
	expires := now.Add(15 * time.Minute)
	if _, err := tx.Exec(ctx, `INSERT INTO data_task_review(token_hash,task_id,operator,review,expires_at,created_at)
 VALUES(@hash,@task,@operator,@review,@expires,@now)`, pgx.NamedArgs{"hash": hash[:], paramTask: taskID, paramOperator: actor.ID, "review": review, "expires": expires, paramNow: now}); err != nil {
		return "", err
	}
	if _, err := storekit.LogSystem(ctx, tx, "data_task.review_issued", map[string]any{"task_id": taskID, paramOperator: actor.ID, "expires_at": expires}); err != nil {
		return "", err
	}
	return token, nil
}

func consumeReview(ctx context.Context, tx pgx.Tx, taskID, operator string, review ReviewRequest, now time.Time) (ReviewRequest, error) {
	hash := sha256.Sum256([]byte(review.PreviewToken))
	review.PreviewToken = ""
	var consumed int
	err := tx.QueryRow(ctx, `DELETE FROM data_task_review WHERE token_hash=@hash AND task_id=@task
 AND operator=@operator AND expires_at>@now AND review=@review::jsonb RETURNING 1`,
		pgx.NamedArgs{"hash": hash[:], paramTask: taskID, paramOperator: operator, paramNow: now, "review": review}).Scan(&consumed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewRequest{}, apperrors.ErrConflict
	}
	if err != nil {
		return ReviewRequest{}, err
	}
	review.Trusted = true
	return review, nil
}

func purgeExpiredReviews(ctx context.Context, tx pgx.Tx, now time.Time) error {
	_, err := tx.Exec(ctx, `DELETE FROM data_task_review WHERE token_hash IN
 (SELECT token_hash FROM data_task_review WHERE expires_at<=@now ORDER BY expires_at LIMIT 1000)`, pgx.NamedArgs{paramNow: now})
	return err
}

func validSealedReview(taskID string, review ReviewRequest, now time.Time) bool {
	return ValidTaskID(taskID) && review.Limit >= 1 && review.Limit <= 1000 && len(review.Scope) <= 262144 && json.Valid(review.Scope) && !review.AsOf.IsZero() && !review.AsOf.After(now) && review.Digest != "" && review.PreviewToken == ""
}
