// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The subject-key rung reads on the CALLER'S transaction, and the proof is a
// pool with one connection in it.
//
// The project-attribution ladder runs inside a transaction. Its third rung asks
// which project a subject's tokens name, through a seam the projects module
// implements — and that implementation opened a connection of its OWN while the
// ladder's transaction was still held. Each caller then holds one connection and
// waits for another: with a small pool, or whenever concurrent captures occupy
// every connection, they deadlock until the context is cancelled, and a path
// documented as never failing the capture stalls the sync instead (#2107).
//
// One connection is the whole fixture. Before the fix this test hangs to its
// deadline; after it, the read is on the transaction already in hand and there
// is no second connection to wait for. The two rungs beside it always read this
// way — this was the one that did not.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheSubjectKeyRungNeedsNoSecondConnection(t *testing.T) {
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` " +
			"(integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(context.Background(), ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	ws := ids.New[ids.WorkspaceKind]()
	if _, err := owner.Exec(context.Background(), `INSERT INTO workspace (id) VALUES ($1)`, ws.UUID); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(appDSN)
	if err != nil {
		t.Fatalf("parsing the app DSN: %v", err)
	}
	// MaxConns alone, for the reason keyvault's own single-connection fixture
	// gives: MinConns is the owner constructor's to set.
	cfg.MaxConns = 1
	pool, err := testdb.OwnPoolFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("opening the single-connection pool: %v", err)
	}
	t.Cleanup(pool.Close)

	store := projects.NewStore(database.BindTo(pool, ws))

	// A deadline rather than waiting for the suite's: a deadlock here is the
	// FINDING, and a test that reports it as "the package timed out" sends the
	// reader to the wrong place.
	reader := principal.WithActor(principal.WithWorkspaceID(context.Background(), ws.UUID),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:probe", UserID: ids.NewV7(),
			Permissions: principal.Permissions{
				Objects:  map[string]principal.ObjectGrant{"project": {Read: true}},
				RowScope: principal.RowScopeAll,
			},
		})
	ctx, cancel := context.WithTimeout(reader, 20*time.Second)
	defer cancel()

	var matched ids.UUID
	err = store.Tx(ctx, func(tx pgx.Tx) error {
		// The rung, called the way the ladder calls it: inside a transaction
		// this pool's only connection is already serving.
		var inner error
		matched, inner = store.MatchProjectKey(ctx, tx, []string{"no-such-key"})
		return inner
	})
	if err != nil {
		t.Fatalf("the subject-key rung could not run inside the caller's transaction: %v — with one "+
			"connection in the pool, an implementation reaching for a second waits for the one it is "+
			"already inside", err)
	}
	if !matched.IsZero() {
		t.Errorf("a key no project carries matched %v", matched)
	}
}
