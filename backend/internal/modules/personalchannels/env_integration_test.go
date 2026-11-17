// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package personalchannels

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// channelEnv is one workspace with two reps and a store over the app role.
type channelEnv struct {
	owner      *pgx.Conn
	store      *Store
	ws         ids.UUID
	rep, other ids.UUID
	now        time.Time
}

func setupChannels(t *testing.T) *channelEnv {
	t.Helper()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := &channelEnv{
		owner: owner, ws: ids.NewV7(), rep: ids.NewV7(), other: ids.NewV7(),
		now: time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC),
	}
	e.exec(t, `INSERT INTO workspace (id) VALUES ($1)`, e.ws)
	for _, u := range []ids.UUID{e.rep, e.other} {
		e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`, u, "rep-"+u.String()+"@channels.test")
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws))).WithClock(func() time.Time { return e.now })
	return e
}

func (e *channelEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *channelEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// as is user with the seeded rep grant on personal_channel, or with grant.
func (e *channelEnv) as(user ids.UUID, grant principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{rbacObject: grant},
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

var repGrant = principal.ObjectGrant{Create: true, Read: true, Update: true}

func (e *channelEnv) system() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: SweepActor,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}
