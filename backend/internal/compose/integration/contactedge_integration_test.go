// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The contact↔contact projection against a real database: the fold, its
// audience rule, the erasure drop's reach on both endpoint columns, and the
// read's row scope. All four claims are SQL — a unit test with hand-built
// rows can fail on none of them.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedSharedThread seeds one activity carrying BOTH contacts (and our
// colleague) as participants, then folds the projections the way the
// cg:graph-edge consumer does.
func seedSharedThread(t *testing.T, e *Env, colleague, personA, personB ids.UUID, subject, audience string) {
	t.Helper()
	owner := OwnerConn(t)
	ctx := context.Background()
	id := ids.NewV7()
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', $2, now(), 'outbound', 'manual', 'human:x', $3)`,
		id, subject, audience); err != nil {
		t.Fatalf("seeding the shared thread: %v", err)
	}
	LinkActivity(t, owner, id, "person", personA)
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'from')`,
		id, colleague); err != nil {
		t.Fatalf("seeding our side: %v", err)
	}
	for _, person := range []ids.UUID{personA, personB} {
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, person_id, role) VALUES ($1, $2, 'to')`,
			id, person); err != nil {
			t.Fatalf("seeding a contact participant: %v", err)
		}
	}
	wsCtx := principal.WithWorkspaceID(ctx, e.WS)
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(wsCtx, tx, []ids.UUID{id})
	}); err != nil {
		t.Fatalf("folding the edges: %v", err)
	}
}

func contactEdgeCount(t *testing.T, a, b ids.UUID) int {
	t.Helper()
	first, second := a, b
	if second.String() < first.String() {
		first, second = second, first
	}
	var n int
	err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT count(*) FROM graph_contact_edge WHERE person_a = $1 AND person_b = $2`,
		first, second).Scan(&n)
	if err != nil {
		t.Fatalf("counting contact edges: %v", err)
	}
	return n
}

// Two externals on one workspace-audience thread are one observed edge,
// stored once in canonical order; a limited-audience thread contributes
// nothing, because who talked to whom on it is content.
func TestASharedThreadFoldsToOneContactEdgeAndALimitedOneToNone(t *testing.T) {
	e := Setup(t)
	anna := e.SeedPerson(t, "Anna Weber", &e.Rep1)
	birgit := e.SeedPerson(t, "Birgit Sommer", &e.Rep1)
	seedSharedThread(t, e, e.Rep1, anna, birgit, "the workspace thread", "workspace")
	if n := contactEdgeCount(t, anna, birgit); n != 1 {
		t.Errorf("workspace thread → %d contact edges, want exactly 1 canonical row", n)
	}

	carla := e.SeedPerson(t, "Carla Nguyen", &e.Rep1)
	seedSharedThread(t, e, e.Rep1, anna, carla, "the limited thread", "participants")
	if n := contactEdgeCount(t, anna, carla); n != 0 {
		t.Errorf("limited-audience thread → %d contact edges, want 0", n)
	}
}

// The erasure drop reaches BOTH endpoint columns: whichever side of the pair
// the subject stands on, the row is gone.
func TestDroppingAPersonRemovesTheirContactEdgesOnEitherEnd(t *testing.T) {
	e := Setup(t)
	anna := e.SeedPerson(t, "Anna Weber", &e.Rep1)
	birgit := e.SeedPerson(t, "Birgit Sommer", &e.Rep1)
	seedSharedThread(t, e, e.Rep1, anna, birgit, "the shared thread", "workspace")

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	// The subject on the LEXICALLY GREATER side, so a person_a-only delete —
	// the copy-paste this test exists to refuse — would leave the row.
	subject := anna
	if birgit.String() > anna.String() {
		subject = birgit
	}
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.DropEdgesForPerson(wsCtx, tx, subject)
	}); err != nil {
		t.Fatalf("dropping the subject's edges: %v", err)
	}
	if n := contactEdgeCount(t, anna, birgit); n != 0 {
		t.Errorf("after the drop the pair still holds %d rows, want 0", n)
	}
}

// A peer outside the caller's row scope is absent from the answer, and the
// caller's own peer is present — the refusal alone cannot prove the read
// works, so the admission rides the same call.
func TestContactPeersCarryOnlyContactsInsideRowScope(t *testing.T) {
	e := Setup(t)
	anchor := e.SeedPerson(t, "Anna Weber", &e.Rep1)
	visible := e.SeedPerson(t, "Birgit Sommer", &e.Rep1)
	hidden := e.SeedPerson(t, "Their Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "person", hidden, e.Rep3)
	seedSharedThread(t, e, e.Rep1, anchor, visible, "the open thread", "workspace")
	seedSharedThread(t, e, e.Rep1, anchor, hidden, "the private contact's thread", "workspace")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	var peers []search.PeerEdge
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		peers, err = search.ContactEdgesForPerson(rep, tx, anchor, 10)
		return err
	})
	if err != nil {
		t.Fatalf("reading the anchor's peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peers = %d, want only the readable one", len(peers))
	}
	if peers[0].Peer != visible {
		t.Errorf("peer is %s, want %s", peers[0].Peer, visible)
	}
	if peers[0].FullName != "Birgit Sommer" {
		t.Errorf("peer named %q, want the person record's own name", peers[0].FullName)
	}
}

// The graph endpoint carries the peer arm: the anchor's observed acquaintance
// arrives as a `peer` node with an anchor↔peer edge, and one human stays one
// node when the peer is already drawn by another arm.
func TestPersonGraphDrawsThePeerArm(t *testing.T) {
	e := Setup(t)
	anchor := e.SeedPerson(t, "Anna Weber", &e.Rep1)
	peer := e.SeedPerson(t, "Birgit Sommer", &e.Rep1)
	seedSharedThread(t, e, e.Rep1, anchor, peer, "the shared thread", "workspace")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	code, graph := readGraph(rep, t, e, anchor)
	if code != 200 {
		t.Fatalf("graph → %d, want 200", code)
	}
	peerNodes := 0
	for _, n := range graph.Nodes {
		if string(n.Group) == "peer" {
			peerNodes++
		}
	}
	if peerNodes != 1 {
		t.Fatalf("peer nodes = %d, want the one observed acquaintance", peerNodes)
	}
	anchorID := "person:" + anchor.String()
	peerID := "person:" + peer.String()
	found := false
	for _, edge := range graph.Edges {
		if (edge.From == anchorID && edge.To == peerID) || (edge.From == peerID && edge.To == anchorID) {
			found = true
		}
	}
	if !found {
		t.Errorf("no anchor↔peer edge in %d edges, want one", len(graph.Edges))
	}
	if graph.DroppedCount == nil || graph.DroppedCount.Peer == nil {
		t.Errorf("dropped_count.peer absent, want it stated (zero included)")
	}
}
