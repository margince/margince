// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestASecondGraphFoldNeverClaimsAnUnwrittenPair(t *testing.T) {
	for _, test := range []struct {
		table   string
		parties int
	}{
		{"graph_contact_edge", 2},
		{"graph_interaction_edge", 1},
	} {
		t.Run(test.table, func(t *testing.T) {
			b := setupBackfillWire(t)
			missing, written := graphContactSets(t, b, test.parties)
			at := clocktest.Now(t).Add(-48 * time.Hour)
			hidden := logHistoryGraphMail(b.env.Admin(), t, b, missing, at)
			if _, err := b.env.Activities.SetAudience(b.env.Admin(), ids.From[ids.ActivityKind](hidden), activities.SetAudienceInput{Audience: "participants"}); err != nil {
				t.Fatal(err)
			}
			visible := logHistoryGraphMail(b.env.Admin(), t, b, written, at)
			system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "system:graph_edge_reconcile")
			ctx, cancel := context.WithTimeout(system, 10*time.Second)
			defer cancel()
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			result := make(chan error, 1)
			var namedMissing atomic.Bool
			go func() {
				defer close(done)
				result <- database.WithWorkspaceTx(ctx, b.env.Pool, func(tx pgx.Tx) error {
					boundary := &graphFoldBarrierTx{Tx: tx, entered: entered, release: release, fold: "INSERT INTO " + test.table + " AS e"}
					boundary.observe = func(args []any) {
						index := 0
						if test.parties == 1 {
							index = 1
						}
						if len(args) > index {
							if keys, ok := args[index].([]ids.UUID); ok && slices.Contains(keys, missing[0]) {
								namedMissing.Store(true)
							}
						}
					}
					return search.RecomputeEdgesForActivities(ctx, boundary, []ids.UUID{hidden, visible})
				})
			}()
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(func() { cancel(); unblock(); <-done })
			select {
			case <-entered:
			case err := <-result:
				t.Fatalf("first fold never acquired its rows: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if !namedMissing.Load() {
				t.Fatal("the first fold did not name the unwritten pair")
			}
			arrival := logHistoryGraphMail(b.env.Admin(), t, b, missing, at.Add(24*time.Hour))
			condition := "contact_id=@first"
			args := pgx.NamedArgs{"first": missing[0]}
			if test.parties == 2 {
				condition = "contact_a=@first AND contact_b=@second"
				args["second"] = missing[1]
			}
			query := `SELECT count(*) FROM ` + pgx.Identifier{test.table}.Sanitize() + ` WHERE ` + condition
			if got := b.env.WsCount(t, query, args); got != 0 {
				t.Fatal("new evidence was already folded before its event")
			}
			unblock()
			if err := <-result; err != nil {
				t.Fatal(err)
			}
			if got := b.env.WsCount(t, query, args); got != 0 {
				t.Fatal("the second fold acquired a pair the first never wrote")
			}
			if err := NewGraphEdgeConsumer(b.env.Pool).HandleEvent(ctx, latestDerivedEvent(t, b.env, "activity.captured", arrival, "")); err != nil {
				t.Fatal(err)
			}
			folded := graphProjectionSnapshot(system, t, b.env.Pool)
			if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { return search.RebuildEdges(system, tx) }); err != nil {
				t.Fatal(err)
			}
			if rebuilt := graphProjectionSnapshot(system, t, b.env.Pool); !maps.Equal(folded, rebuilt) {
				t.Fatalf("event fold and rebuild disagree: folded=%v rebuilt=%v", folded, rebuilt)
			}
		})
	}
}
