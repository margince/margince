// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A close date of 15 June is yesterday in Ho Chi Minh City and today in Berlin
// at 18:30 UTC on the 15th. The same deal is overdue on the Hanoi rep's own list
// and on time on a screen that reads the installation's Berlin day.
func TestADealCloseDateIsOverdueInTheZoneTheScreenReadsIn(t *testing.T) {
	e := setupConfigEnv(t)
	at := time.Date(2026, 6, 15, 18, 30, 0, 0, time.UTC)
	store := e.store.SetClock(func() time.Time { return at })
	open := openStage(t, e, SemanticOpen)
	var pipelineID ids.PipelineID
	if err := store.Tx(e.as(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.as(), `SELECT pipeline_id FROM stage WHERE id = $1`, open).Scan(&pipelineID)
	}); err != nil {
		t.Fatal(err)
	}
	closeOn := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	deal, err := store.CreateDeal(e.asDealWriter(), CreateDealInput{
		Name: "Closes on the 15th", PipelineID: pipelineID, StageID: open, ExpectedClose: &closeOn,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(deal.Id)
	for zone, want := range map[string]bool{"Asia/Ho_Chi_Minh": true, "Europe/Berlin": false} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		figures, err := store.FiguresIn(e.asDealWriter(), []ids.UUID{id}, loc)
		if err != nil {
			t.Fatal(err)
		}
		if got := figures[id].CloseOverdue; got != want {
			t.Errorf("close date of 15 June read in %s: overdue = %t, want %t", zone, got, want)
		}
	}
}
