// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestAllowanceOverflowLockoutRecoversThroughReplaceBudget proves a
// TokensPerFullUser that is valid against today's full-user count can start overflowing
// MaxMonthlyTokens once the workspace's full-user count grows, and — before this fix —
// every admin entry point that could correct it (ReadBudget, PreviewBudget, ReplaceBudget)
// computed the STORED config's MonthlyTokens first and errored before ever looking at the
// correction being submitted.
//
// The growth is driven through the real identity writer (deactivate, then reactivate a
// seat), not a hand-rolled full-user count, and the correction goes through ReplaceBudget
// itself — the actor path an admin's fix actually takes.
func TestAllowanceOverflowLockoutRecoversThroughReplaceBudget(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), budgetFullUsers, aiDeferredWork(e.Pool))
	service := identity.NewService(e.Pool)
	actor := recoveryAdminIdentity(e)
	rep3 := ids.From[ids.UserKind](e.Rep3)

	before, err := store.ReadBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.EligibleFullUsers != 4 {
		t.Fatalf("harness full-user count drifted: %d", before.EligibleFullUsers)
	}

	// A harness-seeded member has no password (never meant to sign in), so
	// ReactivateUser would land them back on 'invited' rather than 'active' —
	// give Rep3 one first so the real reactivate writer below restores a live
	// member, the same as it would for a colleague returning from leave.
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(ctx, `UPDATE app_user SET password_hash = 'test-hash' WHERE id = $1`, e.Rep3); err != nil {
		t.Fatal(err)
	}

	// Shrink to 3 full users, set an allowance valid for exactly that count, then grow
	// back to 4 — the same shape as an admin sizing TokensPerFullUser for today's
	// headcount and a colleague returning from leave later.
	if err := service.DeactivateUser(ctx, actor, identity.DeactivateUserInput{UserID: rep3}); err != nil {
		t.Fatal(err)
	}
	validForThree := ai.MaxMonthlyTokens / 3
	sized, err := store.ReplaceBudget(ctx, ai.BudgetChange{
		Config:           ai.BudgetConfig{TokensPerFullUser: validForThree},
		ExpectedRevision: before.Revision,
	})
	if err != nil {
		t.Fatalf("sizing for 3 full users: %v", err)
	}
	if err := service.ReactivateUser(ctx, actor, rep3); err != nil {
		t.Fatal(err)
	}

	// The workspace is now locked out: validForThree*4 exceeds MaxMonthlyTokens.
	broken, err := store.ReadBudget(ctx)
	if err != nil {
		t.Fatalf("ReadBudget must observe an over-cap config rather than error: %v", err)
	}
	if broken.EligibleFullUsers != 4 || broken.Revision != sized.Revision {
		t.Fatalf("broken snapshot %+v", broken)
	}
	if broken.MonthlyTokens != ai.MaxMonthlyTokens {
		t.Fatalf("an already-stored overflow must saturate at the ceiling for display, got %d", broken.MonthlyTokens)
	}

	// The write-time guard is still live: a NEW value that overflows against the
	// CURRENT full-user count is rejected, not silently accepted because the
	// workspace happened to be broken already.
	_, err = store.ReplaceBudget(ctx, ai.BudgetChange{
		Config:           ai.BudgetConfig{TokensPerFullUser: ai.MaxMonthlyTokens},
		ExpectedRevision: broken.Revision,
	})
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) {
		t.Fatalf("a new overflowing value must be rejected, got %v", err)
	}
	stillBroken, err := store.ReadBudget(ctx)
	if err != nil || stillBroken.Revision != broken.Revision {
		t.Fatalf("a rejected write must not have persisted: %+v %v", stillBroken, err)
	}

	// PreviewBudget is the other half of the recovery: the frontend edit screen
	// disables Save until a preview succeeds, so it must tolerate the same
	// over-cap current config ReadBudget does.
	validForFour := ai.MaxMonthlyTokens / 8
	preview, err := store.PreviewBudget(ctx, ai.BudgetChange{
		Config:           ai.BudgetConfig{TokensPerFullUser: validForFour},
		ExpectedRevision: broken.Revision,
	})
	if err != nil {
		t.Fatalf("PreviewBudget must observe the broken current config, not error on it: %v", err)
	}
	if preview.Current.MonthlyTokens != ai.MaxMonthlyTokens {
		t.Fatalf("preview's current snapshot %+v", preview.Current)
	}
	if preview.Proposed.MonthlyTokens != 4*validForFour {
		t.Fatalf("preview's proposed snapshot %+v", preview.Proposed)
	}

	// The actual recovery: ReplaceBudget accepts the correction end to end, even
	// though the workspace was in the errored state when the call started.
	fixed, err := store.ReplaceBudget(ctx, ai.BudgetChange{
		Config:           ai.BudgetConfig{TokensPerFullUser: validForFour},
		ExpectedRevision: broken.Revision,
	})
	if err != nil {
		t.Fatalf("recovery write must succeed: %v", err)
	}
	if fixed.MonthlyTokens != 4*validForFour {
		t.Fatalf("recovered snapshot %+v", fixed)
	}
	after, err := store.ReadBudget(ctx)
	if err != nil || after.Revision != fixed.Revision || after.MonthlyTokens != fixed.MonthlyTokens {
		t.Fatalf("recovered config did not persist: %+v %v", after, err)
	}
}

// TestAllowanceReadFailsClosedWhenFullUserCountingErrors proves a failure from the
// full-user counter (NewAdminStore's own composition seam) reaches the caller
// rather than being swallowed — a genuine DB-side failure counting seats must
// surface as a read error, not a silent zero that understates the workspace.
func TestAllowanceReadFailsClosedWhenFullUserCountingErrors(t *testing.T) {
	e := integration.Setup(t)
	errCounting := errors.New("full-user count unavailable")
	failingUsers := func(context.Context, pgx.Tx) (int64, error) { return 0, errCounting }
	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), failingUsers, aiDeferredWork(e.Pool))
	if _, err := store.ReadBudget(e.Admin()); !errors.Is(err, errCounting) {
		t.Fatalf("ReadBudget must surface the counting failure, got %v", err)
	}
}
