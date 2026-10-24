// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/owedverdict"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	reviewPredictionUncertain = "uncertain"
	reviewPredictionDeclined  = "declined"
)

const repairReclassifyMessage homeRepairAction = "reclassify_message"

func (c *OwedClassifier) previewHomeRepairVerdicts(ctx context.Context, after ids.UUID, limit int, asOf time.Time) (datatask.ReviewRequest, error) {
	if limit < 1 || limit > 1000 {
		return datatask.ReviewRequest{}, apperrors.ErrInvalidArgument
	}
	var candidates []activities.OwedReviewCandidate
	err := InstallationDB(c.pool).Tx(ctx, func(tx pgx.Tx) error {
		var err error
		candidates, err = activities.OwedReviewPageTx(ctx, tx, after, min(limit, 25), owedBodyLimit, owedPriorBodyLimit)
		return err
	})
	if err != nil {
		return datatask.ReviewRequest{}, err
	}
	predictions, err := c.reviewedVerdictPredictions(ctx, candidates)
	if err != nil {
		return datatask.ReviewRequest{}, err
	}
	scope := homeRepairScope{Tasks: make([]homeRepairTaskRef, 0, len(candidates))}
	for _, candidate := range candidates {
		scope.Tasks = append(scope.Tasks, homeRepairVerdictRef(candidate, predictions[candidate.Candidate.ID]))
	}
	review, err := homeRepairReview(scope, asOf)
	if err != nil || len(scope.Tasks) == 0 {
		return review, err
	}
	err = InstallationDB(c.pool).Tx(ctx, func(tx pgx.Tx) error {
		var err error
		review.PreviewToken, err = datatask.SealReviewTx(ctx, tx, homeRepairTaskID, review, c.now())
		return err
	})
	return review, err
}

func (c *OwedClassifier) reviewedVerdictPredictions(ctx context.Context, candidates []activities.OwedReviewCandidate) (map[ids.UUID]string, error) {
	predictions := make(map[ids.UUID]string, len(candidates))
	if c.brain == nil {
		return predictions, nil
	}
	eligible := make([]owedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Pristine && candidate.ContextReadable && candidate.EvidenceComplete {
			eligible = append(eligible, candidate.Candidate)
		}
	}
	sink := owedPredictionSink{
		assessed: func(msg owedCandidate) {
			if predictions[msg.ID] == "" {
				predictions[msg.ID] = reviewPredictionUncertain
			}
		},
		accept: func(_ context.Context, msg owedCandidate, result owedResult) (bool, error) {
			predictions[msg.ID] = result.Verdict
			return true, nil
		},
		decline: func(_ context.Context, msg owedCandidate, _ error) error {
			predictions[msg.ID] = reviewPredictionDeclined
			return nil
		},
	}
	calls := newSweepCalls(max(len(eligible), owedBatchSize), owedBatchSize)
	for start := 0; start < len(eligible) && calls.remain(); start += owedBatchSize {
		_, err := c.predictBatch(ctx, eligible[start:min(start+owedBatchSize, len(eligible))], calls, sink)
		if sweepPaused(err) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return predictions, nil
}

func homeRepairVerdictRef(candidate activities.OwedReviewCandidate, predicted string) homeRepairTaskRef {
	item := homeRepairTaskRef{Source: repairSourceVerdict, ID: candidate.Candidate.ID, Version: candidate.Version, Snapshot: candidate.Snapshot, BeforeVerdict: candidate.Verdict, Action: repairHumanReview, Reason: "classification_uncertain"}
	if !candidate.Pristine {
		item.Reason = "human_changed_or_provenance_missing"
		return item
	}
	if !candidate.ContextReadable {
		item.Reason = "classification_context_withheld"
		return item
	}
	if !candidate.EvidenceComplete {
		item.Reason = "classification_evidence_incomplete"
		return item
	}
	if predicted == "" {
		item.Reason = "classification_not_attempted"
		return item
	}
	if predicted == reviewPredictionDeclined {
		item.Reason = "classification_declined"
		return item
	}
	if predicted == reviewPredictionUncertain {
		return item
	}
	item.AfterVerdict = predicted
	if predicted == candidate.Verdict {
		item.Action = repairLeaveCurrent
		item.Reason = "classification_current"
		return item
	}
	item.Action = repairReclassifyMessage
	item.Reason = "classification_changed"
	return item
}

func validateHomeRepairVerdictTx(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	candidate, err := activities.OwedReviewTx(ctx, tx, expected.ID, owedBodyLimit, owedPriorBodyLimit)
	if err != nil {
		return err
	}
	if expected.AfterVerdict != "" && !owedVerdicts[expected.AfterVerdict] {
		return apperrors.ErrInvalidArgument
	}
	prediction := expected.AfterVerdict
	if prediction == "" && expected.Reason == "classification_uncertain" {
		prediction = reviewPredictionUncertain
	}
	if prediction == "" && expected.Reason == "classification_declined" {
		prediction = reviewPredictionDeclined
	}
	if actual := homeRepairVerdictRef(candidate, prediction); actual != expected {
		return apperrors.ErrConflict
	}
	return nil
}

func applyHomeRepairVerdict(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, activation datatask.Activation, runID ids.UUID) (bool, error) {
	candidate, err := activities.OwedReviewTx(ctx, tx, expected.ID, owedBodyLimit, owedPriorBodyLimit)
	if err == nil {
		err = validateHomeRepairVerdictTx(ctx, tx, expected)
	}
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrConflict) {
		return false, recordHomeRepairSkip(ctx, tx, expected, classificationSkipReason(err), activation.RuleVersion, runID)
	}
	if err != nil {
		return false, err
	}
	if expected.Action != repairReclassifyMessage {
		return false, recordHomeRepairSkip(ctx, tx, expected, expected.Reason, activation.RuleVersion, runID)
	}
	var wrote bool
	err = pgx.BeginFunc(ctx, tx, func(savepoint pgx.Tx) error {
		var err error
		wrote, err = activities.ApplyReviewedOwedVerdictTx(ctx, savepoint, candidate, expected.AfterVerdict, owedverdict.Ruleset, expected.Reason, runID, owedBodyLimit, owedPriorBodyLimit)
		return err
	})
	if errors.Is(err, apperrors.ErrConflict) || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, recordHomeRepairSkip(ctx, tx, expected, classificationSkipReason(err), activation.RuleVersion, runID)
	}
	return wrote, err
}

func classificationSkipReason(err error) string {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return "classification_permission_lost"
	}
	if errors.Is(err, apperrors.ErrNotFound) {
		return "classification_source_unavailable"
	}
	return "classification_changed_since_review"
}
