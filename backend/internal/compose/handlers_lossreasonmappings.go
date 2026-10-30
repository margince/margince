// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ApplyLegacyLossReasons starts a run of the reviewed loss mappings over the
// decisions as they stand. It needs the right to run a correction and the
// right to decide the mappings it carries.
func (h dataTaskHandlers) ApplyLegacyLossReasons(w http.ResponseWriter, r *http.Request) {
	if !admitOperator(w, r, "data_task", principal.ActionUpdate) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), healthReadTimeout)
	defer cancel()
	if err := auth.Require(ctx, "custom_field", principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	plan, err := h.activateLossMappings(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, crmcontracts.ApplyLegacyLossReasonsResponse{Mappings: plan.mappings, Deals: plan.deals})
}

// activateLossMappings starts the run a plan describes. A plan that changes
// no deal starts nothing.
func (h dataTaskHandlers) activateLossMappings(ctx context.Context) (lossMappingPlan, error) {
	engine, err := newDataTaskEngine(h.pool, buildinfo.ReleaseVersion, h.now)
	if err != nil {
		return lossMappingPlan{}, err
	}
	version, err := lossMappingLedgerVersion(ctx, engine)
	if err != nil {
		return lossMappingPlan{}, err
	}
	var plan lossMappingPlan
	err = InstallationDB(h.pool).Tx(ctx, func(tx pgx.Tx) error {
		var err error
		plan, err = lossMappingReview(ctx, tx, h.now())
		return err
	})
	if err != nil || plan.deals == 0 {
		return plan, err
	}
	if err := engine.Activate(ctx, lossMappingTaskID, lossMappingStep, version, plan.review); err != nil {
		return lossMappingPlan{}, err
	}
	return plan, enqueueDataTaskReconcile(ctx, h.pool)
}

// lossMappingLedgerVersion is the ledger version an activation must name. A
// step the worker has not yet seeded answers 409: there is nothing to start.
func lossMappingLedgerVersion(ctx context.Context, engine *datatask.Engine) (int64, error) {
	progress, err := engine.List(ctx)
	if err != nil {
		return 0, err
	}
	for _, p := range progress {
		if p.TaskID == lossMappingTaskID && p.Step == lossMappingStep {
			return p.Version, nil
		}
	}
	return 0, apperrors.ErrConflict
}
