// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The coalesced cg:graph-edge path (search.GraphEdgeGen.HandleBatch). One
// property carries it: folding a read's events together leaves exactly the
// table folding them one at a time leaves, which is exactly the table the
// nightly rebuild leaves. A batch that disagreed with either would be a second
// fold with its own way of being wrong.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (v edgeEnv) clearProjection(t *testing.T) {
	t.Helper()
	if err := database.WithWorkspaceTx(v.e.Admin(), v.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `DELETE FROM graph_interaction_edge; DELETE FROM graph_contact_edge`)
		return err
	}); err != nil {
		t.Fatalf("clearing the projection: %v", err)
	}
}

func TestABatchFoldsLikeOneEventAtATimeAndLikeTheRebuild(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	now := time.Now().UTC()
	gen := search.NewGraphEdgeGen(search.NewStore(v.e.DB()))
	ctx := context.Background()

	// A mailbox burst: the same colleague on message after message, a second
	// colleague on some of them, a cc, and one message that is later relinked
	// away from the contact it was first filed under.
	was := v.contact(t, "Filed Wrongly")
	c1 := v.contact(t, "Burst One")
	c2 := v.contact(t, "Burst Two")
	var activityIDs []ids.UUID
	for i := range 5 {
		activityIDs = append(activityIDs,
			v.interaction(t, v.e.Rep1, c1, now.AddDate(0, 0, -i), "inbound", "from"),
			v.interaction(t, v.e.Rep1, c2, now.AddDate(0, 0, -i-30), "outbound", "cc"),
			v.interaction(t, v.e.Rep2, c1, now.AddDate(0, 0, -i-100), "outbound", "to"))
	}
	relinked := v.interaction(t, v.e.Rep1, was, now.AddDate(0, 0, -2), "inbound", "from")
	v.recompute(t, append(activityIDs, relinked)...)
	if err := database.WithWorkspaceTx(v.e.Admin(), v.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE activity_participant SET contact_id = $3 WHERE activity_id = $1 AND contact_id = $2`,
			relinked, was, c2)
		return err
	}); err != nil {
		t.Fatalf("relinking: %v", err)
	}

	// The read the consumer would get: every activity event, one delivered
	// twice (at-least-once), the relink's update, and a contact event.
	var envs []kevents.Envelope
	for _, id := range activityIDs {
		envs = append(envs, envelopeFor(v.e.WS, "activity.captured", "activity", id))
	}
	envs = append(envs,
		envelopeFor(v.e.WS, "activity.captured", "activity", activityIDs[0]),
		envelopeFor(v.e.WS, "activity.updated", "activity", relinked),
		envelopeFor(v.e.WS, "contact.updated", "contact", c1.UUID),
		envelopeFor(v.e.WS, "deal.created", "deal", ids.NewV7()))

	if err := gen.HandleBatch(ctx, envs); err != nil {
		t.Fatalf("HandleBatch: %v", err)
	}
	batched := v.snapshot(t)
	if edges := v.edgesFor(t, was); len(edges) != 0 {
		t.Errorf("the batch left the edge to the contact the relinked message LEFT: %+v", edges)
	}

	v.clearProjection(t)
	for _, env := range envs {
		if err := gen.HandleEvent(ctx, env); err != nil {
			t.Fatalf("HandleEvent %s: %v", env.Type, err)
		}
	}
	oneByOne := v.snapshot(t)

	if err := database.WithWorkspaceTx(v.e.Admin(), v.e.Pool, func(tx pgx.Tx) error {
		return search.RebuildEdges(v.e.Admin(), tx)
	}); err != nil {
		t.Fatalf("rebuilding: %v", err)
	}
	rebuilt := v.snapshot(t)

	if len(rebuilt) == 0 {
		t.Fatal("the rebuild produced an empty projection — the comparison would pass vacuously")
	}
	if !equalSnapshots(batched, oneByOne) {
		t.Errorf("the batch disagrees with the per-event path:\n  batched    %v\n  one-by-one %v", batched, oneByOne)
	}
	if !equalSnapshots(batched, rebuilt) {
		t.Errorf("the batch disagrees with the rebuild:\n  batched %v\n  rebuilt %v", batched, rebuilt)
	}
}

func TestABatchWithNothingToFoldTouchesNothing(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	gen := search.NewGraphEdgeGen(search.NewStore(v.e.DB()))
	envs := []kevents.Envelope{
		envelopeFor(v.e.WS, "deal.created", "deal", ids.NewV7()),
		envelopeFor(v.e.WS, "activity.restricted", "activity", ids.NewV7()),
		envelopeFor(v.e.WS, "activity.captured", "activity", ids.Nil),
	}
	if err := gen.HandleBatch(context.Background(), envs); err != nil {
		t.Fatalf("a batch of events this projection ignores errored: %v", err)
	}
	if err := gen.HandleBatch(context.Background(), nil); err != nil {
		t.Fatalf("an empty batch errored: %v", err)
	}
	// The per-entry path a failed batch falls back to ignores the same events.
	if err := gen.HandleEvent(context.Background(), envs[1]); err != nil {
		t.Fatalf("an activity event the projection ignores errored one at a time: %v", err)
	}
}

// A batch that cannot fold must say so, because the error is what sends its
// entries down the per-entry path instead of acking them unfolded.
func TestABatchThatCannotFoldReturnsTheError(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	c := v.contact(t, "Unfolded")
	activity := v.interaction(t, v.e.Rep1, c, time.Now().UTC(), "inbound", "from")
	gen := search.NewGraphEdgeGen(search.NewStore(v.e.DB()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gen.HandleBatch(ctx, []kevents.Envelope{
		envelopeFor(v.e.WS, "activity.captured", "activity", activity),
		envelopeFor(v.e.WS, "contact.updated", "contact", c.UUID),
	}); err == nil {
		t.Fatal("a batch whose transaction could not run reported success, so its entries would be acked unfolded")
	}
	if edges := v.edgesFor(t, c); len(edges) != 0 {
		t.Errorf("a failed batch left %d edges behind", len(edges))
	}
}
