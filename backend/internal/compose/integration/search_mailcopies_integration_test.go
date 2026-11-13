// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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
