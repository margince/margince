// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The activity branch ranks a bounded sample of its matches. Copies of one
// mail that many mailboxes captured must not fill that sample and push a
// distinct matching mail out of it.
//
// The copies are written in one statement, under the key shape capture files a
// seat's copy with. Two thousand rows through the sink would cost minutes.
func TestMailboxCopiesDoNotCrowdADistinctMailOutOfTheRankSample(t *testing.T) {
	e := SetupSearch(t)
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at,
		                      source_system, source_id, source, captured_by)
		SELECT gen_random_uuid(), 'email', 'Hangar plans', 'the hangar plans attached', 'inbound',
		       'jonas@acme.example', '2026-06-04T08:00:00Z', $1, $2 || gen_random_uuid()::text, 'gmail', 'connector:gmail'
		  FROM generate_series(1, 2001)`,
		connector.EmailSourceSystem, connector.SeatScopedMailKey("crowded@acme.example", "")); err != nil {
		t.Fatalf("seeding the copies: %v", err)
	}
	distinct := e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at,
		                      source_system, source_id, source, captured_by)
		VALUES ($1, 'email', 'Hangar invoice', 'the hangar invoice', 'inbound', 'billing@acme.example',
		        '2026-06-05T08:00:00Z', $2, 'distinct@acme.example', 'gmail', 'connector:gmail')`, connector.EmailSourceSystem)

	page, err := e.Store.Search(e.Admin(), search.Input{Query: "hangar", Types: []string{"activity"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]int{}
	for _, hit := range page.Hits {
		found[hit.Title]++
		if hit.ID == distinct {
			found["distinct"]++
		}
	}
	if found["distinct"] != 1 || found["Hangar plans"] != 1 {
		t.Errorf("want the distinct mail and one hit for the copies, got %+v", page.Hits)
	}
}

// Meaning-based search groups mailbox copies the way the keyword lane does. A
// reader of two copies of one mail gets one hit: their own copy, though the
// other mailbox filed first. A mail one word apart is no copy.
func TestSimilarSearchShowsMailTwoMailboxesCapturedOnce(t *testing.T) {
	e := SetupSearch(t)
	fake := ai.NewFakeClient()
	embedder := fakeEmbedder(t, fake)
	const text = "Hangar plans the hangar plans attached"
	file := func(key, body string) ids.UUID {
		id := e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at,
		                      source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'Hangar plans', $2, 'inbound', 'jonas@acme.example', '2026-06-04T08:00:00Z',
			        $3, $4, 'gmail', 'connector:gmail')`, body, connector.EmailSourceSystem, key)
		if _, err := e.Store.UpsertEmbedding(e.Admin(), "activity", id, "Hangar plans "+body, embedder); err != nil {
			t.Fatalf("embedding %s: %v", key, err)
		}
		return id
	}
	colleagues := file(connector.SeatScopedMailKey("plans@acme.example", e.Rep3.String()), "the hangar plans attached")
	own := file(connector.SeatScopedMailKey("plans@acme.example", e.Rep1.String()), "the hangar plans attached")
	other := file("other@acme.example", "the hangar plans attached again")

	query, err := fake.Embed(context.Background(), model.EmbedRequest{Inputs: []string{text}})
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := embedder.EmbedIdentity()
	hits, err := e.Store.SimilarEntities(e.AsFullUser(), query.Vectors[0], identity, 10, "activity")
	if err != nil {
		t.Fatal(err)
	}
	got := map[ids.UUID]int{}
	for _, hit := range hits {
		got[hit.ID]++
	}
	if len(hits) != 2 || got[own] != 1 || got[other] != 1 || got[colleagues] != 0 {
		t.Errorf("want one hit for the copies, the reader's own %s, and one for the other mail %s; got %+v", own, other, hits)
	}
}

// More copies of one mail than the first grouping pass samples still leave
// room for the next distinct mail. The pass that may have cut it runs again.
func TestSimilarSearchFindsTheNextMailPastManyCopiesOfOne(t *testing.T) {
	e := SetupSearch(t)
	fake := ai.NewFakeClient()
	embedder := fakeEmbedder(t, fake)
	const limit = 2
	file := func(key, body string) ids.UUID {
		id := e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at,
		                      source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'Hangar plans', $2, 'inbound', 'jonas@acme.example', '2026-06-04T08:00:00Z',
			        $3, $4, 'gmail', 'connector:gmail')`, body, connector.EmailSourceSystem, key)
		if _, err := e.Store.UpsertEmbedding(e.Admin(), "activity", id, "Hangar plans "+body, embedder); err != nil {
			t.Fatalf("embedding %s: %v", key, err)
		}
		return id
	}
	copies := connector.FoldBounds(limit)[0] + 1
	for i := range copies {
		file(connector.SeatScopedMailKey("plans@acme.example", fmt.Sprintf("seat-%d", i)), "the hangar plans attached")
	}
	other := file("other@acme.example", "the hangar plans attached again")

	query, err := fake.Embed(context.Background(), model.EmbedRequest{Inputs: []string{"Hangar plans the hangar plans attached"}})
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := embedder.EmbedIdentity()
	hits, err := e.Store.SimilarEntities(e.AsFullUser(), query.Vectors[0], identity, limit, "activity")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[1].ID != other {
		t.Errorf("want one hit for the %d copies and then the other mail %s; got %+v", copies, other, hits)
	}
}
