// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An embed call is a provider round trip of up to ai.CallCeiling, and no
// transaction may stay open across it: an idle-in-transaction session holds a
// pool slot, pins the vacuum horizon database-wide, and is killed by
// database.IdleTransactionCeiling, throwing away a model call already paid for.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// lockProbeEmbedder asks, from inside Embed, whether any transaction still
// holds a lock on the embedding table. The upsert's own read takes ACCESS
// SHARE, which lasts until its transaction ends, so an ACCESS EXCLUSIVE NOWAIT
// from another session is refused exactly while that transaction is open.
type lockProbeEmbedder struct {
	search.Embedder
	owner    *pgx.Conn
	probeErr error
}

func (p *lockProbeEmbedder) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	p.probeErr = pgx.BeginFunc(ctx, p.owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `LOCK TABLE embedding IN ACCESS EXCLUSIVE MODE NOWAIT`)
		return err
	})
	return p.Embedder.Embed(ctx, req)
}

func TestUpsertHoldsNoTransactionAcrossTheEmbedCall(t *testing.T) {
	e := SetupSearch(t)
	contactID := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Span Contact', 'manual', 'human:x')`)
	probe := &lockProbeEmbedder{Embedder: fakeEmbedder(t, ai.NewFakeClient()), owner: e.Owner}

	for _, text := range []string{"Span Contact", "Span Contact renamed"} {
		fresh, err := e.Store.UpsertEmbedding(e.Admin(), "contact", contactID, text, probe)
		if err != nil || !fresh {
			t.Fatalf("upsert of %q: fresh=%v err=%v", text, fresh, err)
		}
		var pgErr *pgconn.PgError
		if errors.As(probe.probeErr, &pgErr) && pgErr.Code == lockNotAvailable {
			t.Fatalf("upsert of %q held a transaction open across the embed call", text)
		}
		if probe.probeErr != nil {
			t.Fatalf("probing the embedding table's locks: %v", probe.probeErr)
		}
	}
}

// racingEmbedder lands a concurrent writer's row while the upsert is between
// its read and its write — the window the hash CAS exists to close.
type racingEmbedder struct {
	search.Embedder
	owner    *pgx.Conn
	entityID ids.UUID
	raceErr  error
}

func (r *racingEmbedder) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	res, err := r.Embedder.Embed(ctx, req)
	if err != nil {
		return res, err
	}
	_, r.raceErr = r.owner.Exec(ctx, `
		UPDATE embedding SET chunk_hash = 'won-the-race'
		 WHERE entity_type = 'contact' AND entity_id = $1 AND chunk_ix = 0`, r.entityID)
	return res, nil
}

func TestUpsertYieldsToAWriterThatLandedDuringTheEmbedCall(t *testing.T) {
	e := SetupSearch(t)
	embedder := fakeEmbedder(t, ai.NewFakeClient())
	contactID := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Race Contact', 'manual', 'human:x')`)
	if fresh, err := e.Store.UpsertEmbedding(e.Admin(), "contact", contactID, "Race Contact", embedder); err != nil || !fresh {
		t.Fatalf("seeding upsert: fresh=%v err=%v", fresh, err)
	}

	racer := &racingEmbedder{Embedder: embedder, owner: e.Owner, entityID: contactID}
	fresh, err := e.Store.UpsertEmbedding(e.Admin(), "contact", contactID, "Race Contact renamed", racer)
	if racer.raceErr != nil {
		t.Fatalf("landing the concurrent write: %v", racer.raceErr)
	}
	if err != nil {
		t.Fatalf("losing the race is not an error, got %v", err)
	}
	if fresh {
		t.Fatal("an upsert that lost the race must report fresh=false")
	}
	var hash string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT chunk_hash FROM embedding WHERE entity_type = 'contact' AND entity_id = $1 AND chunk_ix = 0`,
		contactID).Scan(&hash); err != nil {
		t.Fatalf("reading the stored hash: %v", err)
	}
	if hash != "won-the-race" {
		t.Fatalf("the concurrent writer's row was clobbered: chunk_hash = %q", hash)
	}
}
