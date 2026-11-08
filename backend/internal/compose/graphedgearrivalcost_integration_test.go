// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCapturedMailDoesNotRefoldUnrelatedCorrespondents(t *testing.T) {
	for _, batched := range []bool{false, true} {
		t.Run(fmt.Sprintf("batched=%t", batched), func(t *testing.T) {
			v := edgeEnv{integration.Setup(t)}
			ctx := context.Background()
			owner := integration.OwnerConn(t)
			at := clocktest.Now(t).Add(-48 * time.Hour)
			var activities []ids.UUID
			var target ids.ContactID
			for i := range 32 {
				contact := v.contact(t, fmt.Sprintf("Correspondent %d", i))
				activities = append(activities, v.interaction(t, v.e.Rep1, contact, at, "inbound", "from"))
				if i == 0 {
					target = contact
				}
			}
			v.recompute(t, activities...)
			if _, err := owner.Exec(ctx, `UPDATE graph_interaction_edge SET computed_at='2000-01-01T00:00:00Z'`); err != nil {
				t.Fatal(err)
			}
			added := v.interaction(t, v.e.Rep1, target, at.Add(24*time.Hour), "inbound", "from")
			event := envelopeFor(v.e.WS, "activity.captured", "activity", added)
			gen := search.NewGraphEdgeGen(search.NewStore(v.e.DB()))
			if batched {
				if err := gen.HandleBatch(ctx, []kevents.Envelope{event, event}); err != nil {
					t.Fatal(err)
				}
			} else if err := gen.HandleEvent(ctx, event); err != nil {
				t.Fatal(err)
			}
			var untouched int
			if err := owner.QueryRow(ctx, `SELECT count(*) FROM graph_interaction_edge WHERE computed_at='2000-01-01T00:00:00Z'`).Scan(&untouched); err != nil {
				t.Fatal(err)
			}
			if untouched != 31 {
				t.Fatalf("untouched unrelated pairs=%d want=31", untouched)
			}
			edges := v.edgesFor(t, target)
			if len(edges) != 1 || edges[0].CountTotal != 2 {
				t.Fatalf("target edges=%+v want one edge counting two activities", edges)
			}
		})
	}
}
