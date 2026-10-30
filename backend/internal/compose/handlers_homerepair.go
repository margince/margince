// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (h dataTaskHandlers) PreviewHomeRepair(w http.ResponseWriter, r *http.Request) {
	if !admitOperator(w, r, "data_task", principal.ActionRead) {
		return
	}
	var in crmcontracts.PreviewHomeRepairJSONBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	timeout := healthReadTimeout
	if in.Source != nil && string(*in.Source) == repairSourceVerdict {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	after := ids.Nil
	if in.After != nil {
		after = ids.UUID(*in.After)
	}
	source := repairSourceTask
	if in.Source != nil {
		source = string(*in.Source)
	}
	var review datatask.ReviewRequest
	var err error
	if source == repairSourceVerdict {
		classifier := h.classifier
		if classifier == nil {
			classifier = NewOwedClassifier(h.pool, nil, h.now, slog.Default())
		}
		review, err = classifier.previewHomeRepairVerdicts(ctx, after, in.Limit, h.now())
	} else {
		err = InstallationDB(h.pool).Tx(ctx, func(tx pgx.Tx) error {
			var err error
			review, err = previewHomeRepairSourceTx(ctx, tx, source, after, in.Limit, h.now())
			return err
		})
	}

	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	out, err := homeRepairReviewWire(review)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h dataTaskHandlers) ActivateHomeRepair(w http.ResponseWriter, r *http.Request) {
	if !admitOperator(w, r, "data_task", principal.ActionUpdate) {
		return
	}
	var in crmcontracts.ActivateHomeRepairJSONBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	if err := admitReviewedRepairActions(in); err != nil {
		httperr.Write(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), healthReadTimeout)
	defer cancel()
	review, err := homeRepairReviewInput(in.Review)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	engine, err := newDataTaskEngine(h.pool, buildinfo.ReleaseVersion, h.now)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := engine.Activate(ctx, homeRepairTaskID, "reviewed_tasks", in.Version, review); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := enqueueDataTaskReconcile(ctx, h.pool); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func homeRepairReviewWire(review datatask.ReviewRequest) (crmcontracts.HomeRepairReview, error) {
	var scope homeRepairScope
	if err := json.Unmarshal(review.Scope, &scope); err != nil {
		return crmcontracts.HomeRepairReview{}, err
	}
	out := crmcontracts.HomeRepairReview{AsOf: review.AsOf, Digest: review.Digest, Limit: review.Limit, RuleVersion: review.RuleVersion, Tasks: make([]crmcontracts.HomeRepairTask, 0, len(scope.Tasks))}
	if review.PreviewToken != "" {
		out.PreviewToken = &review.PreviewToken
	}
	revalidate, normalize := 0, 0
	for _, task := range scope.Tasks {
		item := crmcontracts.HomeRepairTask{Id: openapi_types.UUID(task.ID), Version: task.Version, Action: crmcontracts.HomeRepairTaskAction(task.Action), Reason: task.Reason}
		if task.BeforeVerdict != "" {
			item.BeforeVerdict = new(crmcontracts.HomeRepairTaskBeforeVerdict(task.BeforeVerdict))
		}
		if task.AfterVerdict != "" {
			item.AfterVerdict = new(crmcontracts.HomeRepairTaskAfterVerdict(task.AfterVerdict))
		}
		if task.Source != "" {
			item.Source = new(crmcontracts.HomeRepairTaskSource(task.Source))
		}
		if task.Snapshot != "" {
			item.Snapshot = &task.Snapshot
		}
		out.Tasks = append(out.Tasks, item)
		if !legacyHomeRepairAction(task.Action) {
			out.RequiresActionReview = new(true)
		}
		switch task.Action {
		case repairReclassifyMessage:
			out.Counts.Reclassify++
		case repairNormalizeTask:
			normalize++
		case repairArchiveReminder:
			out.Counts.Archive++
		case repairWithdrawProposal:
			out.Counts.Withdraw++
		case repairSupersedeSuggestion:
			out.Counts.Supersede++
		case repairHumanReview:
			out.Counts.Review++
		case repairLeaveCurrent:
			out.Counts.Keep++
		case repairRevalidateSettlement:
			revalidate++
		default:
			return crmcontracts.HomeRepairReview{}, apperrors.ErrInvalidArgument
		}
	}
	if normalize > 0 {
		out.Counts.Normalize = &normalize
	}
	if revalidate > 0 {
		out.Counts.Revalidate = &revalidate
	}
	return out, nil
}

func homeRepairReviewInput(in crmcontracts.HomeRepairReview) (datatask.ReviewRequest, error) {
	scope := homeRepairScope{Tasks: make([]homeRepairTaskRef, 0, len(in.Tasks))}
	for _, task := range in.Tasks {
		item := homeRepairTaskRef{ID: ids.UUID(task.Id), Version: task.Version, Action: homeRepairAction(task.Action), Reason: task.Reason}
		if task.BeforeVerdict != nil {
			item.BeforeVerdict = string(*task.BeforeVerdict)
		}
		if task.AfterVerdict != nil {
			item.AfterVerdict = string(*task.AfterVerdict)
		}
		if task.Source != nil {
			item.Source = string(*task.Source)
		}
		if task.Snapshot != nil {
			item.Snapshot = *task.Snapshot
		}
		scope.Tasks = append(scope.Tasks, item)
	}
	encoded, err := json.Marshal(scope)
	review := datatask.ReviewRequest{Scope: encoded, Digest: in.Digest, Limit: in.Limit, RuleVersion: in.RuleVersion, AsOf: in.AsOf}
	if in.PreviewToken != nil {
		review.PreviewToken = *in.PreviewToken
	}
	return review, err
}
