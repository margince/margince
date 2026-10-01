// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAMergeDropsTheSourceEdgesAndRefoldsTheSurvivor(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	gen := search.NewGraphEdgeGen(search.NewStore(v.e.DB()))
	source := v.contact(t, "Merge Source")
	survivor := v.contact(t, "Merge Survivor")
	activity := v.interaction(t, v.e.Rep1, source, time.Now().UTC(), "inbound", "from")
	v.recompute(t, activity)
	if len(v.edgesFor(t, source)) != 1 {
		t.Fatal("the seed produced no edge to the merge source")
	}

	// The merge repoints the activity link before its event is handled.
	if _, err := owner.Exec(ctx, `UPDATE activity_participant SET contact_id = $2 WHERE contact_id = $1`, source, survivor); err != nil {
		t.Fatalf("repointing the participant: %v", err)
	}
	env := envelopeFor(v.e.WS, "contact.merged", "contact", source.UUID)
	env.Payload = []byte(fmt.Sprintf(`{"merged_into_id":%q}`, survivor.UUID))
	if err := gen.HandleEvent(ctx, env); err != nil {
		t.Fatalf("HandleEvent contact.merged: %v", err)
	}

	if edges := v.edgesFor(t, source); len(edges) != 0 {
		t.Errorf("the merge source kept its edges: %+v", edges)
	}
	if edges := v.edgesFor(t, survivor); len(edges) != 1 || edges[0].CountTotal != 1 {
		t.Errorf("the survivor's edge was not refolded from the repointed activity: %+v", edges)
	}
}

func TestRefoldingAContactRestoresItsEdgesAndDroppingRemovesThem(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	owner := integration.OwnerConn(t)
	c := v.contact(t, "Refold Subject")
	other := v.contact(t, "Refold Peer")
	activity := v.interaction(t, v.e.Rep1, c, time.Now().UTC(), "inbound", "from")
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'cc')`, activity, other); err != nil {
		t.Fatalf("adding a peer to the activity: %v", err)
	}
	v.recompute(t, activity)
	v.clearProjection(t)

	if err := v.inTx(t, func(tx pgx.Tx) error { return search.RecomputeEdgesForContact(v.e.Admin(), tx, c.UUID) }); err != nil {
		t.Fatalf("RecomputeEdgesForContact: %v", err)
	}
	if edges := v.edgesFor(t, c); len(edges) != 1 || edges[0].CountTotal != 1 {
		t.Errorf("refolding the contact did not restore its edge: %+v", edges)
	}
	if edges := v.edgesFor(t, other); len(edges) != 0 {
		t.Errorf("refolding one contact rebuilt its peer's edge as well: %+v", edges)
	}

	if err := v.inTx(t, func(tx pgx.Tx) error { return search.DropEdgesForContact(v.e.Admin(), tx, c.UUID) }); err != nil {
		t.Fatalf("DropEdgesForContact: %v", err)
	}
	if edges := v.edgesFor(t, c); len(edges) != 0 {
		t.Errorf("dropping the contact left interaction edges: %+v", edges)
	}
	var peerRows int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM graph_contact_edge WHERE contact_a = $1 OR contact_b = $1`, c).Scan(&peerRows); err != nil {
		t.Fatalf("counting peer edges: %v", err)
	}
	if peerRows != 0 {
		t.Errorf("dropping the contact left %d contact-to-contact edges", peerRows)
	}
}

func (v edgeEnv) inTx(t *testing.T, fn func(pgx.Tx) error) error {
	t.Helper()
	return database.WithWorkspaceTx(v.e.Admin(), v.e.Pool, fn)
}

// A write the database refuses must surface, whichever table it fails on, so
// the bus retries the entry instead of acking a projection it did not fold.
func TestAProjectionWriteTheDatabaseRefusesSurfaces(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	c := v.contact(t, "Refused Subject")
	activity := v.interaction(t, v.e.Rep1, c, time.Now().UTC(), "inbound", "from")
	v.recompute(t, activity)

	breakingTable := func(t *testing.T, table string, run func(tx pgx.Tx) error) error {
		t.Helper()
		tx, err := owner.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() {
			if err := tx.Rollback(ctx); err != nil {
				t.Errorf("rolling back the broken table: %v", err)
			}
		}()
		if _, err := tx.Exec(ctx, `ALTER TABLE `+table+` RENAME TO `+table+`_gone`); err != nil {
			t.Fatalf("breaking %s: %v", table, err)
		}
		return run(tx)
	}
	cases := []struct {
		name, table string
		run         func(tx pgx.Tx) error
	}{
		{
			"refolding a contact whose colleagues cannot be read", "graph_interaction_edge",
			func(tx pgx.Tx) error { return search.RecomputeEdgesForContact(ctx, tx, c.UUID) },
		},
		{
			"refolding a contact whose peers cannot be read", "graph_contact_edge",
			func(tx pgx.Tx) error { return search.RecomputeEdgesForContact(ctx, tx, c.UUID) },
		},
		{
			"dropping a contact's peer edges", "graph_contact_edge",
			func(tx pgx.Tx) error { return search.DropEdgesForContact(ctx, tx, c.UUID) },
		},
		{
			"dropping a contact's interaction edges", "graph_interaction_edge",
			func(tx pgx.Tx) error { return search.DropEdgesForContact(ctx, tx, c.UUID) },
		},
		{
			"refolding activities", "graph_contact_edge",
			func(tx pgx.Tx) error { return search.RecomputeEdgesForActivities(ctx, tx, []ids.UUID{activity}) },
		},
		{
			"rebuilding", "graph_interaction_edge",
			func(tx pgx.Tx) error { return search.RebuildEdges(ctx, tx) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := breakingTable(t, tc.table, tc.run); err == nil {
				t.Errorf("%s with %s missing returned nil", tc.name, tc.table)
			}
		})
	}
}

func TestRefoldingNoActivitiesTouchesNothing(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	if err := v.inTx(t, func(tx pgx.Tx) error { return search.RecomputeEdgesForActivities(v.e.Admin(), tx, nil) }); err != nil {
		t.Fatalf("an empty refold errored: %v", err)
	}
}
