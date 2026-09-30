// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/collections"
)

// With lists switched off the Live List check records nothing, so no list
// history grows and no list.evaluated event leaves; switched on, it checks.
func TestTheLiveListCheckRunsOnlyWhileListsAreOn(t *testing.T) {
	e := integration.Setup(t)
	list, err := NewCollectionsStore(e.Pool).CreateList(e.Admin(), collections.CreateListInput{
		Name: "Everyone", EntityType: "contact", ListType: "dynamic", Sharing: "workspace",
		Definition: map[string]any{"field": "title", "op": "eq", "value": "Buyer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	checked := func() int {
		t.Helper()
		var n int
		if err := e.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM list_evaluation WHERE list_id = $1`, list.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, on := range []bool{false, true} {
		worker := &listEvaluateWorker{enabled: on, pool: e.Pool, now: time.Now, log: slog.New(slog.DiscardHandler)}
		if err := worker.Work(context.Background(), nil); err != nil {
			t.Fatalf("lists on=%v: %v", on, err)
		}
		if got := checked(); (got > 0) != on {
			t.Fatalf("lists on=%v: %d checks recorded", on, got)
		}
	}
}
