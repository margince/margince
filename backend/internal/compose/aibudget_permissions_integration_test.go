// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAllowanceReadingsPreserveTheirIndependentPermissions(t *testing.T) {
	e := integration.Setup(t)
	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), budgetFullUsers, aiDeferredWork(e.Pool))
	for _, routing := range []bool{false, true} {
		perms := principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"ai_budget": {Read: true}, "ai_diagnostics": {Read: true}, "ai_routing": {Read: routing},
		}}
		status, err := store.ReadStatus(e.As(e.Rep1, nil, perms))
		if err != nil {
			t.Fatal(err)
		}
		if (len(status.Features) > 0) != routing || (status.RoutingVersion != "") != routing {
			t.Fatalf("routing=%v: features=%d revision=%q", routing, len(status.Features), status.RoutingVersion)
		}
		if len(status.DeferredWork) != 3 {
			t.Fatal("diagnostics reader lost its carrier readings")
		}
	}
	perms := principal.Permissions{Objects: map[string]principal.ObjectGrant{"ai_diagnostics": {Read: true}}}
	if _, err := store.ReadStatus(e.As(e.Rep1, nil, perms)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("diagnostics alone exposed allowance configuration: %v", err)
	}
	perms.Objects = map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}}
	if _, err := store.PreviewRouting(e.As(e.Rep1, nil, perms), ai.RoutingConfig{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("routing preview without allowance read: %v", err)
	}
	perms.Objects = map[string]principal.ObjectGrant{"ai_budget": {Read: true, Update: true}}
	ctx := e.As(e.Rep1, nil, perms)
	budget, err := store.ReadBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewBudget(ctx, ai.BudgetChange{Config: ai.BudgetConfig{TokensPerFullUser: 13000000}, ExpectedRevision: budget.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Features) != 0 || len(preview.DeferredWork) != 0 || preview.Proposed.Config.TokensPerFullUser != 13000000 {
		t.Fatalf("budget-only preview crossed its grant boundary: %+v", preview)
	}
}
