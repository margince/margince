// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAllowanceWriteIsLiveAuditedAndRevisionGuarded(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), budgetFullUsers, aiDeferredWork(e.Pool))
	before, err := store.ReadBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := ai.BudgetConfig{TokensPerFullUser: 123456}
	change := ai.BudgetChange{Config: config, ExpectedRevision: before.Revision}
	preview, err := store.PreviewBudget(ctx, change)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Proposed.MonthlyTokens != before.BudgetedFullUsers*config.TokensPerFullUser {
		t.Fatalf("preview %+v", preview)
	}
	still, err := store.ReadBudget(ctx)
	if err != nil || still.Revision != before.Revision {
		t.Fatalf("preview wrote: %+v %v", still, err)
	}
	after, err := store.ReplaceBudget(ctx, change)
	if err != nil {
		t.Fatal(err)
	}
	live, err := NewSeatBudget(e.Pool).MonthlyTokenBudget(context.Background(), ids.From[ids.WorkspaceKind](e.WS))
	if err != nil || live != after.MonthlyTokens {
		t.Fatalf("runtime %d, saved %d: %v", live, after.MonthlyTokens, err)
	}
	if _, err := store.ReplaceBudget(ctx, change); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale save: %v", err)
	}
	change.ExpectedRevision = after.Revision
	if _, err := store.ReplaceBudget(ctx, change); err != nil {
		t.Fatal(err)
	}
	err = e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var count int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM event_outbox e JOIN audit_log a ON a.id=(e.envelope->'trace'->>'audit_log_id')::uuid WHERE e.envelope->>'type'='ai_budget.updated'`).Scan(&count)
		if err == nil && count != 1 {
			t.Errorf("%d correlated events, want one real change", count)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentAllowanceEditsHaveOneWinner(t *testing.T) {
	e := integration.Setup(t)
	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), budgetFullUsers, aiDeferredWork(e.Pool))
	before, err := store.ReadBudget(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, tokens := range []int64{1000, 2000} {
		group.Go(func() {
			_, err := store.ReplaceBudget(e.Admin(), ai.BudgetChange{ExpectedRevision: before.Revision, Config: ai.BudgetConfig{TokensPerFullUser: tokens}})
			results <- err
		})
	}
	group.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, apperrors.ErrVersionSkew):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestAllowanceAndAuditRollBackWhenItsEventCannotCommit(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), budgetFullUsers, aiDeferredWork(e.Pool))
	before, err := store.ReadBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(ctx, `ALTER TABLE event_outbox ADD CONSTRAINT refuse_budget_test CHECK (envelope->>'type' <> 'ai_budget.updated') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `ALTER TABLE event_outbox DROP CONSTRAINT refuse_budget_test`); err != nil {
			t.Error(err)
		}
	})
	_, err = store.ReplaceBudget(ctx, ai.BudgetChange{ExpectedRevision: before.Revision, Config: ai.BudgetConfig{TokensPerFullUser: 1234}})
	if err == nil {
		t.Fatal("outbox refusal did not abort the change")
	}
	after, err := store.ReadBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatal("setting escaped transaction rollback")
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var count int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type='ai_budget' AND after ? 'ai.budget'`).Scan(&count)
		if err == nil && count != 0 {
			t.Errorf("rollback left %d audits", count)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
