// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

func TestAllowanceTriggerFiltersOtherEventsAndCoalescesPendingRecovery(t *testing.T) {
	e := integration.Setup(t)
	runner, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	trigger := AIBudgetResumeTrigger(runner)
	count := func() int {
		t.Helper()
		var count int
		if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
			return tx.QueryRow(e.Admin(), `SELECT count(*) FROM river_job WHERE kind='ai_budget_resume'`).Scan(&count)
		}); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if err := trigger(e.Admin(), events.Envelope{Type: "contact.updated"}); err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Fatal("unrelated event queued recovery")
	}
	for range 2 {
		if err := trigger(e.Admin(), events.Envelope{Type: "ai_budget.updated"}); err != nil {
			t.Fatal(err)
		}
	}
	if count() != 1 {
		t.Fatal("duplicate events queued duplicate active recovery")
	}
}

func TestUnavailableCarrierReadingDoesNotClaimZeroWaitingWork(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(t.Context(), `ALTER TABLE voice_build RENAME TO voice_build_unavailable_probe`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `ALTER TABLE voice_build_unavailable_probe RENAME TO voice_build`); err != nil {
			t.Errorf("restore carrier: %v", err)
		}
	})
	readings, err := aiDeferredWork(e.Pool)(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 3 {
		t.Fatal("missing carrier reading")
	}
	for _, reading := range readings {
		if reading.Carrier == "voice_build" {
			if reading.Available || reading.Count != nil {
				t.Fatal("failed carrier read claimed a count")
			}
		} else if !reading.Available || reading.Count == nil {
			t.Fatalf("one failed carrier hid another: %+v", reading)
		}
	}
}
