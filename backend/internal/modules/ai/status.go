// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// observed is the read surfaces' snapshot: it never errors on a stored budget that
// has grown past the overflow ceiling (see AdminStore.observedTx), so a workspace
// already over-cap can still be read and corrected. Real spend (compose's
// seatBudget.MonthlyTokenBudget) stays on BudgetConfig.MonthlyTokens's strict,
// fail-closed contract — only observing an already-stored value tolerates the
// overflow, never authorizing more of it.
func (s *AdminStore) observed(ctx context.Context) (crmcontracts.AiBudgetSnapshot, RoutingConfig, error) {
	var budget crmcontracts.AiBudgetSnapshot
	var cfg RoutingConfig
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		budget, err = s.observedTx(ctx, tx)
		if err != nil {
			return err
		}
		if auth.Require(ctx, routingSettingsObject, principal.ActionRead) == nil {
			cfg, err = settings.GetTx(ctx, tx, Routing)
		}
		return err
	})
	return budget, cfg, err
}

// ReadStatus reports prospective routing separately from observed provider health.
func (s *AdminStore) ReadStatus(ctx context.Context) (crmcontracts.AiStatus, error) {
	if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
		return crmcontracts.AiStatus{}, err
	}
	if err := auth.Require(ctx, budgetObject, principal.ActionRead); err != nil {
		return crmcontracts.AiStatus{}, err
	}
	budget, cfg, err := s.observed(ctx)
	if err != nil {
		return crmcontracts.AiStatus{}, err
	}
	deferred, err := s.visibleDeferred(ctx)
	if err != nil {
		return crmcontracts.AiStatus{}, err
	}
	unused := unusedTiers(cfg)
	version := ""
	if auth.Require(ctx, routingSettingsObject, principal.ActionRead) == nil {
		version = cfg.Revision()
	}
	return crmcontracts.AiStatus{
		UnusedTiers: &unused, Budget: budget, ObservedAt: budget.ObservedAt, RoutingVersion: version, TaskContractHash: TaskContractHash,
		Features: visibleFeatureRoutes(ctx, cfg, budget.Band), DeferredWork: deferred, DeferredWorkCoverage: "durable_builds_and_scans",
	}, nil
}

// PreviewBudget evaluates a draft without reserving capacity or starting work.
func (s *AdminStore) PreviewBudget(ctx context.Context, change BudgetChange) (crmcontracts.AiBudgetPreview, error) {
	if err := auth.Require(ctx, budgetObject, principal.ActionRead); err != nil {
		return crmcontracts.AiBudgetPreview{}, err
	}
	if err := auth.Require(ctx, budgetObject, principal.ActionUpdate); err != nil {
		return crmcontracts.AiBudgetPreview{}, err
	}
	current, cfg, err := s.observed(ctx)
	if err != nil {
		return crmcontracts.AiBudgetPreview{}, err
	}
	if current.Revision != change.ExpectedRevision {
		return crmcontracts.AiBudgetPreview{}, apperrors.ErrVersionSkew
	}
	proposed, err := previewBudget(current, change.Config)
	if err != nil {
		return crmcontracts.AiBudgetPreview{}, err
	}
	deferred, err := s.visibleDeferred(ctx)
	if err != nil {
		return crmcontracts.AiBudgetPreview{}, err
	}
	return crmcontracts.AiBudgetPreview{Current: current, Proposed: proposed, Features: visibleFeatureRoutes(ctx, cfg, proposed.Band), DeferredWork: deferred}, nil
}

// PreviewRouting compares both bindings under the same current allowance band.
func (s *AdminStore) PreviewRouting(ctx context.Context, next RoutingConfig) (crmcontracts.AiRoutingPreview, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	if err := auth.Require(ctx, budgetObject, principal.ActionRead); err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	if err := validateStoredRouting(next); err != nil {
		return crmcontracts.AiRoutingPreview{}, settings.InvalidValue{Setting: RoutingKey, Code: settings.CodeInvalidValue, Reason: err.Error()}
	}
	budget, cfg, err := s.observed(ctx)
	if err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	return crmcontracts.AiRoutingPreview{CurrentVersion: cfg.Revision(), Features: compareFeatureRoutes(cfg, next, budget.Band, budget.Band), UnusedTiers: unusedTiers(next)}, nil
}

// Budget-only editors can preview their policy without gaining diagnostics or
// routing access. Omitted readings remain absent rather than being fetched via
// the machinery-only settings path.
func (s *AdminStore) visibleDeferred(ctx context.Context) ([]crmcontracts.AiDeferredWork, error) {
	if auth.Require(ctx, "ai_diagnostics", principal.ActionRead) == nil {
		return s.deferred(ctx)
	}
	return []crmcontracts.AiDeferredWork{}, nil
}

func visibleFeatureRoutes(ctx context.Context, cfg RoutingConfig, band string) []crmcontracts.AiFeatureRoute {
	if auth.Require(ctx, routingSettingsObject, principal.ActionRead) != nil {
		return []crmcontracts.AiFeatureRoute{}
	}
	return FeatureRoutes(cfg, cfg, band)
}
