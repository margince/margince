// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const homeRepairVersion = "1:" + activities.CommitmentAdmissionVersion

type homeRepairScope struct {
	Tasks []homeRepairTaskRef `json:"tasks"`
}

type homeRepairTaskRef struct {
	ID      ids.UUID         `json:"id"`
	Version int64            `json:"version"`
	Action  homeRepairAction `json:"action"`
	Reason  string           `json:"reason"`
}

func previewHomeRepairTx(ctx context.Context, tx pgx.Tx, limit int, asOf time.Time) (datatask.ReviewRequest, error) {
	return previewHomeRepairAfterTx(ctx, tx, ids.Nil, limit, asOf)
}

func previewHomeRepairAfterTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, asOf time.Time) (datatask.ReviewRequest, error) {
	if limit < 1 || limit > 1000 {
		return datatask.ReviewRequest{}, apperrors.ErrInvalidArgument
	}
	scope := homeRepairScope{Tasks: make([]homeRepairTaskRef, 0, limit)}
	for len(scope.Tasks) < limit {
		decisions, err := homeRepairTaskDecisionsTx(ctx, tx, after, limit-len(scope.Tasks), asOf)
		if err != nil {
			return datatask.ReviewRequest{}, err
		}
		if len(decisions) == 0 {
			break
		}
		for _, decision := range decisions {
			scope.Tasks = append(scope.Tasks, homeRepairTaskRef{ID: decision.Task.ID.UUID, Version: decision.Task.Version, Action: decision.Action, Reason: decision.Reason})
			after = decision.Task.ID.UUID
		}
	}
	encoded, err := json.Marshal(scope)
	if err != nil {
		return datatask.ReviewRequest{}, err
	}
	review := datatask.ReviewRequest{Scope: encoded, RuleVersion: homeRepairVersion, Limit: len(scope.Tasks), AsOf: asOf}
	review.Digest, err = homeRepairDigest(review)
	return review, err
}

func homeRepairDigest(review datatask.ReviewRequest) (string, error) {
	review.Digest = ""
	encoded, err := json.Marshal(review)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validateHomeRepairTx(ctx context.Context, tx pgx.Tx, review datatask.ReviewRequest) (string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionDelete); err != nil {
		return "", err
	}
	var scope homeRepairScope
	if err := json.Unmarshal(review.Scope, &scope); err != nil {
		return "", apperrors.ErrInvalidArgument
	}
	if len(scope.Tasks) != review.Limit {
		return "", apperrors.ErrInvalidArgument
	}
	seen := make(map[ids.UUID]bool, len(scope.Tasks))
	for _, expected := range scope.Tasks {
		if seen[expected.ID] {
			return "", apperrors.ErrInvalidArgument
		}
		seen[expected.ID] = true
		task, err := activities.HomeRepairTaskTx(ctx, tx, expected.ID)
		if err != nil {
			return "", err
		}
		decision, err := homeRepairTaskDecisionTx(ctx, tx, task, review.AsOf)
		if err != nil {
			return "", err
		}
		if task.Version != expected.Version || decision.Action != expected.Action || decision.Reason != expected.Reason {
			return "", apperrors.ErrConflict
		}
	}
	return homeRepairDigest(review)
}
