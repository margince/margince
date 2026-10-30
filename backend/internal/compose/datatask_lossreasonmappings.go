// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	lossMappingTaskID        = "core/1793359670_loss_reason_mappings"
	lossMappingStep          = "mapped_losses"
	lossMappingReviewVersion = "1"
	// maxReviewedLossMappings is the most decisions one run applies, and
	// maxReviewedLossDeals the most deals it changes: the engine bounds a
	// reviewed run at 1000 records. The next apply takes the rest.
	maxReviewedLossMappings = 1000
	maxReviewedLossDeals    = 1000
)

// lossMappingScope is the reviewed plan: each decided mapping at the version
// the administrator applied.
type lossMappingScope struct {
	Mappings []deals.ReviewedLossMapping `json:"mappings"`
}

// lossMappingDataTask carries an administrator's reviewed mappings of legacy
// loss notes onto the lost deals that carry them. It is manual: each apply
// starts a new run over the decisions as they stand. A run changes only what
// an earlier one left different.
func lossMappingDataTask(pool *pgxpool.Pool) datatask.Task {
	return datatask.Task{
		ID: lossMappingTaskID, Version: 1, Title: "Applying reviewed loss reasons",
		Manual: true, ReviewVersion: lossMappingReviewVersion, Review: validateLossMappingReviewTx,
		Requires: []string{"core/1793359670"},
		Steps: []datatask.Step{{
			Name: lossMappingStep, Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateLossMappings,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return applyLossMappingBatch(ctx, tx, pool, in)
			},
		}},
	}
}

func applyLossMappingBatch(ctx context.Context, tx pgx.Tx, pool *pgxpool.Pool, in datatask.BatchIn) (datatask.BatchOut, error) {
	if in.Activation == nil {
		return datatask.BatchOut{}, apperrors.ErrPermissionDenied
	}
	human, ok := principal.HumanUserID(in.Activation.Operator)
	if !ok {
		return datatask.BatchOut{}, apperrors.ErrPermissionDenied
	}
	actor, err := (storedActor{authority: identity.NewService(pool)}).rebuild(ctx,
		migration.StoredActor{Type: principal.PrincipalHuman, UserID: human})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	ctx = principal.WithActor(ctx, actor)
	if err := auth.Require(ctx, "data_task", principal.ActionUpdate); err != nil {
		return datatask.BatchOut{}, err
	}
	var scope lossMappingScope
	if err := json.Unmarshal(in.Activation.Scope, &scope); err != nil {
		return datatask.BatchOut{}, apperrors.ErrInvalidArgument
	}
	reviewed := make(map[ids.UUID]int64, len(scope.Mappings))
	for _, mapping := range scope.Mappings {
		reviewed[mapping.ID] = mapping.Version
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned, changed, err := deals.ApplyLossReasonMappingsTx(ctx, tx, after, in.Limit, reviewed)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

func estimateLossMappings(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := deals.CountLossMappingCandidates(ctx, q, maximum, nil)
	return min(count, maximum), count > maximum, err
}

// lossMappingPlan is what one apply will do: the decisions it carries and how
// many lost deals they change, which bounds the run.
type lossMappingPlan struct {
	review   datatask.ReviewRequest
	mappings int
	deals    int
}

// lossMappingReview is the plan an apply activates: every decision as it
// stands now, sealed by its digest.
func lossMappingReview(ctx context.Context, tx pgx.Tx, asOf time.Time) (lossMappingPlan, error) {
	mappings, err := deals.ReviewedLossMappingsTx(ctx, tx, maxReviewedLossMappings)
	if err != nil {
		return lossMappingPlan{}, err
	}
	reviewed := make([]ids.UUID, 0, len(mappings))
	for _, mapping := range mappings {
		reviewed = append(reviewed, mapping.ID)
	}
	pending, err := deals.CountLossMappingCandidates(ctx, tx, maxReviewedLossDeals, reviewed)
	if err != nil {
		return lossMappingPlan{}, err
	}
	plan := lossMappingPlan{mappings: len(mappings), deals: int(min(pending, maxReviewedLossDeals))}
	encoded, err := json.Marshal(lossMappingScope{Mappings: mappings})
	if err != nil {
		return lossMappingPlan{}, err
	}
	plan.review = datatask.ReviewRequest{Scope: encoded, RuleVersion: lossMappingReviewVersion, Limit: max(plan.deals, 1), AsOf: asOf}
	plan.review.Digest, err = reviewDigest[lossMappingScope](plan.review)
	return plan, err
}

// validateLossMappingReviewTx rechecks the plan on the activating
// transaction: every mapping it names still stands at the version reviewed.
func validateLossMappingReviewTx(ctx context.Context, tx pgx.Tx, review datatask.ReviewRequest) (string, error) {
	var scope lossMappingScope
	if err := json.Unmarshal(review.Scope, &scope); err != nil || len(scope.Mappings) > maxReviewedLossMappings {
		return "", apperrors.ErrInvalidArgument
	}
	current, err := deals.ReviewedLossMappingsTx(ctx, tx, maxReviewedLossMappings)
	if err != nil {
		return "", err
	}
	versions := make(map[ids.UUID]int64, len(current))
	for _, mapping := range current {
		versions[mapping.ID] = mapping.Version
	}
	for _, mapping := range scope.Mappings {
		if version, ok := versions[mapping.ID]; !ok || version != mapping.Version {
			return "", apperrors.ErrConflict
		}
	}
	return reviewDigest[lossMappingScope](review)
}
