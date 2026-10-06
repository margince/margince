// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// No transaction stays open across an embed call; UpsertEmbedding says why.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// txProbeEmbedder counts, from inside Embed, the sessions sitting idle in a
// transaction on this package's database. Each integration package runs on its
// own clone, and a test that is not parallel never runs beside another in its
// package, so any such session is the upsert's own.
type txProbeEmbedder struct {
	search.Embedder
	owner    *pgx.Conn
	openTxs  int
	probeErr error
}

func (p *txProbeEmbedder) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	p.probeErr = p.owner.QueryRow(ctx, `
		SELECT count(*) FROM pg_stat_activity
		 WHERE datname = current_database() AND pid <> pg_backend_pid()
		   AND state LIKE 'idle in transaction%'`).Scan(&p.openTxs)
	return p.Embedder.Embed(ctx, req)
}

func TestUpsertHoldsNoTransactionAcrossTheEmbedCall(t *testing.T) {
	e := SetupSearch(t)
	// pg_stat_activity hides another role's state from an unprivileged reader,
	// which would read as zero open transactions and pass for the wrong reason.
	var canSeeState bool
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT pg_has_role(current_user, 'pg_read_all_stats', 'MEMBER')`).Scan(&canSeeState); err != nil || !canSeeState {
		t.Fatalf("the owner role must read other sessions' state (pg_read_all_stats): can=%v err=%v", canSeeState, err)
	}
	contactID := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Span Contact', 'manual', 'human:x')`)
	probe := &txProbeEmbedder{Embedder: fakeEmbedder(t, ai.NewFakeClient()), owner: e.Owner}

	for _, text := range []string{"Span Contact", "Span Contact renamed"} {
		fresh, err := e.Store.UpsertEmbedding(e.Admin(), "contact", contactID, text, probe)
		if err != nil || !fresh {
			t.Fatalf("upsert of %q: fresh=%v err=%v", text, fresh, err)
		}
		if probe.probeErr != nil {
			t.Fatalf("reading pg_stat_activity: %v", probe.probeErr)
		}
		if probe.openTxs != 0 {
			t.Fatalf("upsert of %q held %d transaction(s) open across the embed call", text, probe.openTxs)
		}
	}
}

// racingEmbedder lands a concurrent writer's change while the upsert is
// between its read and its write — the window the stamp CAS exists to close.
type racingEmbedder struct {
	search.Embedder
	owner    *pgx.Conn
	entityID ids.UUID
	race     string
	raceErr  error
}

func (r *racingEmbedder) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	res, err := r.Embedder.Embed(ctx, req)
	if err != nil {
		return res, err
	}
	_, r.raceErr = r.owner.Exec(ctx, r.race, r.entityID)
	return res, nil
}

func TestUpsertYieldsToAWriterThatLandedDuringTheEmbedCall(t *testing.T) {
	fake := ai.NewFakeClient()
	cases := []struct {
		name, race, column, want string
		seeded                   bool
		text                     string
		embedder                 func(t *testing.T) search.Embedder
	}{
		{
			name: "a newer text", column: "chunk_hash", want: "won-the-race", seeded: true,
			race: `UPDATE embedding SET chunk_hash = 'won-the-race'
			        WHERE entity_type = 'contact' AND entity_id = $1 AND chunk_ix = 0`,
			text:     "Race Contact renamed",
			embedder: func(t *testing.T) search.Embedder { return fakeEmbedder(t, fake) },
		},
		{
			name: "the same text under another model", column: "model", want: "fake/won-the-race@1024", seeded: true,
			race: `UPDATE embedding SET model = 'fake/won-the-race@1024'
			        WHERE entity_type = 'contact' AND entity_id = $1 AND chunk_ix = 0`,
			text:     "Race Contact",
			embedder: func(t *testing.T) search.Embedder { return fakeEmbedderNamed(t, fake, "model-swapped") },
		},
		{
			name: "a first vector for an entity that had none", column: "chunk_hash", want: "won-the-race",
			race: `INSERT INTO embedding (entity_type, entity_id, chunk_ix, chunk_hash, model, embedding)
			       VALUES ('contact', $1, 0, 'won-the-race', 'fake/won-the-race@3', '[1,2,3]')`,
			text:     "Race Contact",
			embedder: func(t *testing.T) search.Embedder { return fakeEmbedder(t, fake) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := SetupSearch(t)
			contactID := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Race Contact', 'manual', 'human:x')`)
			if tc.seeded {
				if fresh, err := e.Store.UpsertEmbedding(e.Admin(), "contact", contactID, "Race Contact", fakeEmbedder(t, fake)); err != nil || !fresh {
					t.Fatalf("seeding upsert: fresh=%v err=%v", fresh, err)
				}
			}

			racer := &racingEmbedder{Embedder: tc.embedder(t), owner: e.Owner, entityID: contactID, race: tc.race}
			fresh, err := e.Store.UpsertEmbedding(e.Admin(), "contact", contactID, tc.text, racer)
			if racer.raceErr != nil {
				t.Fatalf("landing the concurrent write: %v", racer.raceErr)
			}
			if err != nil {
				t.Fatalf("losing the race is not an error, got %v", err)
			}
			if fresh {
				t.Fatal("an upsert that lost the race must report fresh=false")
			}
			var got string
			if err := e.Owner.QueryRow(context.Background(),
				`SELECT `+tc.column+` FROM embedding WHERE entity_type = 'contact' AND entity_id = $1 AND chunk_ix = 0`,
				contactID).Scan(&got); err != nil {
				t.Fatalf("reading the stored %s: %v", tc.column, err)
			}
			if got != tc.want {
				t.Fatalf("the concurrent writer's row was clobbered: %s = %q", tc.column, got)
			}
		})
	}
}
