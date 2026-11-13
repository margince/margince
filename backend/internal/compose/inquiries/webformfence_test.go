// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWebFormFencesOverOnePoolShareOneSessionBudget(t *testing.T) {
	pool := &pgxpool.Pool{}
	over := func(p *pgxpool.Pool) *database.DB { return database.BindTo(p, ids.WorkspaceID{}) }
	intakeFence, routingFence := NewWebFormFence(over(pool), nil), NewWebFormFence(over(pool), nil)
	if intakeFence.budgets != routingFence.budgets {
		t.Fatal("two web form fences draw on two budget registries")
	}

	budgets := &webFormBudgets{}
	fence := func(p *pgxpool.Pool) *WebFormFence {
		return &WebFormFence{db: over(p), budgets: budgets, capacity: func() int { return 1 }}
	}
	intake, routing := fence(pool), fence(pool)
	intake.sessionSlots() <- struct{}{}
	select {
	case routing.sessionSlots() <- struct{}{}:
		t.Fatal("a second caller over the same pool took a slot past the pool's bound")
	default:
	}
	select {
	case fence(&pgxpool.Pool{}).sessionSlots() <- struct{}{}:
	default:
		t.Fatal("a fence over another pool shares this pool's budget")
	}
}
