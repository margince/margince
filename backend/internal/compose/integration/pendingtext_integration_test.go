// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

// The pending count reads an activity's text only for rows that lack an
// embedding, so an embedded row with a large body must neither count nor
// contribute its length, while an unembedded one counts with its full text.
func TestPendingCountsOnlyUnembeddedActivitiesAndTheirText(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()
	const identity = "fake/pending-text@1024"
	if err := e.Store.SeedBinding(ctx, identity); err != nil {
		t.Fatalf("SeedBinding: %v", err)
	}

	big := strings.Repeat("quarterly renewal terms ", 200)
	embedded := e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by) VALUES ($1, 'email', 'covered', '`+big+`', 'manual', 'human:x')`)
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO embedding (entity_type, entity_id, chunk_ix, chunk_hash, model, embedding)
		VALUES ('activity', $1, 0, 'h', $2, '[1,2,3]'::vector)`, embedded, identity); err != nil {
		t.Fatalf("seeding the covered activity's embedding: %v", err)
	}
	e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by) VALUES ($1, 'email', 'open', 'abcd', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by) VALUES ($1, 'email', ' ', '  ', 'manual', 'human:x')`)

	pending, err := e.Store.EntitiesPending(ctx, identity)
	if err != nil {
		t.Fatalf("EntitiesPending: %v", err)
	}
	if pending != 1 {
		t.Fatalf("EntitiesPending = %d, want 1 (the unembedded activity; not the covered one, not the blank one)", pending)
	}

	tokens, err := e.Store.TokenSumByWorkspace(ctx, identity)
	if err != nil {
		t.Fatalf("TokenSumByWorkspace: %v", err)
	}
	if len(tokens) == 0 {
		t.Fatal("TokenSumByWorkspace returned no workspaces — the per-workspace assertion would pass vacuously")
	}
	for ws, got := range tokens {
		// "open abcd" is 9 bytes, 2 tokens at 4 bytes each; the covered body would be ~1200.
		if got != 2 {
			t.Errorf("tokens[%v] = %d, want 2 (only the unembedded activity's text)", ws, got)
		}
	}
}
