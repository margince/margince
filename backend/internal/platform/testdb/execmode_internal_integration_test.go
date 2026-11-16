// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestEveryPooledDSNRunsProductionsExecMode covers every pool this package has
// handed out, not the one a test happened to ask for.
//
// A mode without a statement cache plans every statement on every call. On a
// statement carrying the content gate, that costs far more than running it. So
// the lane stays on the mode cmd/api runs. The two DSNs come from different
// env vars, so a mode exported on one would escape an app-pool-only assertion.
func TestEveryPooledDSNRunsProductionsExecMode(t *testing.T) {
	t.Parallel()
	// Both DSNs, so the map holds what the lane's fixtures hold.
	db := For(t)
	for _, dsn := range []string{db.AppDSN, db.OwnerDSN} {
		if _, err := Pool(context.Background(), dsn); err != nil {
			t.Fatalf("opening the shared pool for %s: %v", redactDSN(dsn), err)
		}
	}

	poolsMu.Lock()
	defer poolsMu.Unlock()
	if len(pools) == 0 {
		t.Fatal("no pooled DSN to check — this gate would pass on a package that never shared a pool")
	}
	for dsn, pool := range pools {
		if got := pool.Config().ConnConfig.DefaultQueryExecMode; got != pgx.QueryExecModeCacheStatement {
			t.Errorf("the shared pool for %s runs exec mode %v, want cache_statement: a mode without a cache "+
				"re-plans every statement and is most of the lane's time", redactDSN(dsn), got)
		}
	}
}
