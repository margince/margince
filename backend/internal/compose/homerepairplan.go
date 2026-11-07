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

	"github.com/margince/margince/backend/internal/compose/lifecyclereview"
	"github.com/margince/margince/backend/internal/compose/owedverdict"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var homeRepairVersion = "3:" + activities.SettlementRepairVersion + ":" + activities.CommitmentAdmissionVersion + ":" + deals.SuggestionRepairVersion + ":" + lifecyclereview.RuleVersion + ":" + owedverdict.Ruleset

const (
	repairSourceTask       = "task"
	repairSourceApproval   = "approval"
	repairSourceSuggestion = "suggestion"
	repairSourceVerdict    = "verdict"
	repairSourceSettlement = "settlement"
	repairCurrentWork      = "current_work"
	repairRecordID         = "record_id"
)

type homeRepairScope struct {
	Tasks []homeRepairTaskRef `json:"tasks"`
}

type homeRepairTaskRef struct {
	BeforeVerdict string           `json:"before_verdict,omitempty"`
	AfterVerdict  string           `json:"after_verdict,omitempty"`
	Source        string           `json:"source,omitempty"`
	Snapshot      string           `json:"snapshot,omitempty"`
	ID            ids.UUID         `json:"id"`
	Version       int64            `json:"version"`
	Action        homeRepairAction `json:"action"`
	Reason        string           `json:"reason"`
}

func previewHomeRepairAfterTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, asOf time.Time) (datatask.ReviewRequest, error) {
	return previewHomeRepairSourceTx(ctx, tx, repairSourceTask, after, limit, asOf)
}

func previewHomeRepairSourceTx(ctx context.Context, tx pgx.Tx, source string, after ids.UUID, limit int, asOf time.Time) (datatask.ReviewRequest, error) {
	if limit < 1 || limit > 1000 {
		return datatask.ReviewRequest{}, apperrors.ErrInvalidArgument
	}
	if source == repairSourceSuggestion {
		return previewHomeRepairSuggestionsTx(ctx, tx, after, limit, asOf)
	}
	if source == repairSourceSettlement {
		return previewHomeRepairSettlementsTx(ctx, tx, after, limit, asOf)
	}
	scope := homeRepairScope{Tasks: make([]homeRepairTaskRef, 0, limit)}
	if source == repairSourceApproval {
		proposals, err := approvals.RepairPageTx(ctx, tx, after, limit, asOf)
		if err != nil {
			return datatask.ReviewRequest{}, err
		}
		for _, proposal := range proposals {
			item, err := homeRepairProposalDecisionTx(ctx, tx, proposal, asOf, false)
			if err != nil {
				return datatask.ReviewRequest{}, err
			}
			scope.Tasks = append(scope.Tasks, item)
		}
		return homeRepairReview(scope, asOf)
	}
	if source != repairSourceTask {
		return datatask.ReviewRequest{}, apperrors.ErrInvalidArgument
	}
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
	return homeRepairReview(scope, asOf)
}

func homeRepairReview(scope homeRepairScope, asOf time.Time) (datatask.ReviewRequest, error) {
	encoded, err := json.Marshal(scope)
	if err != nil {
		return datatask.ReviewRequest{}, err
	}
	review := datatask.ReviewRequest{Scope: encoded, RuleVersion: homeRepairVersion, Limit: len(scope.Tasks), AsOf: asOf}
	review.Digest, err = homeRepairDigest(review)
	return review, err
}

func homeRepairDigest(review datatask.ReviewRequest) (string, error) {
	return reviewDigest[homeRepairScope](review)
}

// reviewDigest names a reviewed plan by its canonical form, so a scope that
// decodes to the same plan always has the same digest. S is the task's scope.
func reviewDigest[S any](review datatask.ReviewRequest) (string, error) {
	var scope S
	if err := json.Unmarshal(review.Scope, &scope); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(scope)
	if err != nil {
		return "", err
	}
	review.Scope = canonical
	review.Digest = ""
	review.PreviewToken = ""
	encoded, err := json.Marshal(review)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validateHomeRepairTx(ctx context.Context, tx pgx.Tx, review datatask.ReviewRequest) (string, error) {
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
		if err := validateHomeRepairItemTx(ctx, tx, expected, review); err != nil {
			return "", err
		}
	}
	return homeRepairDigest(review)
}

func validateHomeRepairItemTx(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, review datatask.ReviewRequest) error {
	if expected.Source == repairSourceVerdict {
		if !review.Trusted {
			return apperrors.ErrPermissionDenied
		}
		return validateHomeRepairVerdictTx(ctx, tx, expected)
	}
	if expected.Source == repairSourceSuggestion {
		return validateHomeRepairSuggestionTx(ctx, tx, expected, review.AsOf)
	}
	if expected.Source == repairSourceSettlement {
		return validateHomeRepairSettlementTx(ctx, tx, expected, review.AsOf)
	}
	if expected.Source == repairSourceApproval {
		return validateHomeRepairProposalTx(ctx, tx, expected, review.AsOf, false)
	}
	if expected.Source != "" && expected.Source != repairSourceTask {
		return apperrors.ErrInvalidArgument
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	task, err := activities.HomeRepairTaskTx(ctx, tx, expected.ID)
	if err != nil {
		return err
	}
	decision, err := homeRepairTaskDecisionTx(ctx, tx, task, review.AsOf, true)
	if err != nil {
		return err
	}
	if task.Version != expected.Version || decision.Action != expected.Action || decision.Reason != expected.Reason {
		return apperrors.ErrConflict
	}
	if decision.Action == repairArchiveReminder {
		return auth.Require(ctx, "activity", principal.ActionDelete)
	}
	if decision.Action == repairNormalizeTask {
		return auth.Require(ctx, "activity", principal.ActionUpdate)
	}
	return nil
}
