// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A member whose role cannot record what capture writes cannot connect a
// mailbox. One whose role shrinks after connecting stops at the next sync,
// with no provider call. These run against identity's real resolver and the
// shipped roles, because the roles are what the product decides on.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// seedShippedRoles writes the roles a new installation starts with.
func seedShippedRoles(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := identity.SeedSystemRoles(context.Background(), tx)
		return err
	}); err != nil {
		t.Fatalf("seeding the shipped roles: %v", err)
	}
}

// holdOnlyRole makes the shipped role key the member's one role.
func holdOnlyRole(t *testing.T, e *integration.SearchEnv, user ids.UUID, key string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `DELETE FROM role_assignment WHERE user_id = $1`, user); err != nil {
			return err
		}
		tag, err := tx.Exec(context.Background(),
			`INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = $2`, user, key)
		if err == nil && tag.RowsAffected() != 1 {
			err = errors.New("no shipped role is called " + key)
		}
		return err
	}); err != nil {
		t.Fatalf("giving %s the %s role: %v", user, key, err)
	}
}

// Of the shipped roles, only Read-only cannot capture; every other one holds
// the creates capture's writers make.
func TestTheShippedRolesThatCannotCaptureAreReadOnlyAlone(t *testing.T) {
	e := integration.SetupSearch(t)
	seedShippedRoles(t, e)
	authority := identity.NewService(e.Pool)
	var keys []string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT key FROM role WHERE is_system ORDER BY key`)
		if err != nil {
			return err
		}
		keys, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var refused []string
	for _, key := range keys {
		holdOnlyRole(t, e, e.Rep1, key)
		rbac, err := authority.EffectiveRBAC(e.Admin(), e.WS, e.Rep1)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if capturemod.CaptureRefusal(rbac.Permissions, principal.SeatFull) != nil {
			refused = append(refused, key)
		}
	}
	if len(keys) < 2 || !slices.Equal(refused, []string{"read_only"}) {
		t.Fatalf("of the shipped roles %v, %v cannot capture; want only read_only", keys, refused)
	}
}

// countingConnector counts provider pulls, so a test can prove none happened.
type countingConnector struct {
	moodyConnector
	pulls int
}

func (c *countingConnector) Sync(ctx context.Context, auth connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	c.pulls++
	return c.moodyConnector.Sync(ctx, auth, cursor, sink)
}

func TestConnectRefusesAMemberWhoseRoleCannotCapture(t *testing.T) {
	e := integration.SetupSearch(t)
	seedShippedRoles(t, e)
	holdOnlyRole(t, e, e.Rep1, "read_only")
	registry := capturemod.NewRegistry(e.DB(), capturemod.NewSink(e.DB()), identity.NewService(e.Pool), newTestKeyvault(t, e))
	registry.Register(&countingConnector{name: "gmail"})

	_, err := registry.Connect(humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead}), "gmail", connector.Auth("token"))
	if _, refused := errors.AsType[*capturemod.RoleCannotCaptureError](err); !refused {
		t.Fatalf("Connect as read_only = %v, want the role refusal", err)
	}
	if n := countRows(t, e, `SELECT count(*) FROM capture_connection`); n != 0 {
		t.Errorf("the refused connect stored %d connections, want 0", n)
	}
	var sealed int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM vault_secret`).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if sealed != 0 {
		t.Errorf("the refused connect sealed %d credentials, want 0", sealed)
	}
}

func lastSuccessAt(t *testing.T, e *integration.SearchEnv, connID ids.UUID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT last_success_at FROM capture_sync_state WHERE connection_id = $1`, connID).Scan(&at)
	}); err != nil {
		t.Fatal(err)
	}
	return at
}

func TestAMemberMovedToReadOnlyStopsWithoutAProviderCallAndResumesWhenRestored(t *testing.T) {
	e := integration.SetupSearch(t)
	seedShippedRoles(t, e)
	holdOnlyRole(t, e, e.Rep1, "rep")
	provider := &countingConnector{name: "gmail"}
	registry := capturemod.NewRegistry(e.DB(), capturemod.NewSink(e.DB()), identity.NewService(e.Pool), newTestKeyvault(t, e))
	registry.Register(provider)
	connID, err := registry.Connect(humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead}), "gmail", connector.Auth("token"))
	if err != nil {
		t.Fatalf("Connect as rep: %v", err)
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := registry.SyncOnce(wsCtx, connID); err != nil {
		t.Fatalf("first sync as rep: %v", err)
	}
	succeeded := lastSuccessAt(t, e, connID)

	holdOnlyRole(t, e, e.Rep1, "read_only")
	forceDue(t, e, connID)
	if err := registry.SyncOnce(wsCtx, connID); err == nil {
		t.Fatal("a sync for a member who cannot capture must report its refusal")
	}
	status, row := readSyncState(t, e, connID)
	if provider.pulls != 1 {
		t.Errorf("provider pulls = %d, want 1: the refused sync must not reach the provider", provider.pulls)
	}
	if status != "error" || row.class == nil || *row.class != "forbidden" || row.failures != 1 {
		t.Fatalf("after one refused sync: status=%s class=%v failures=%d, want error/forbidden/1", status, row.class, row.failures)
	}
	if wait := time.Until(row.next); wait < time.Minute || wait > 3*time.Minute {
		t.Errorf("next check in %s, want the two-minute mail interval, not the backoff ladder", wait)
	}
	if got := lastSuccessAt(t, e, connID); got == nil || !got.Equal(*succeeded) {
		t.Errorf("last_success_at moved from %v to %v on a refused sync", succeeded, got)
	}

	forceDue(t, e, connID)
	if err := registry.SyncOnce(wsCtx, connID); err == nil {
		t.Fatal("the second refused sync must report its refusal too")
	}
	if n := countRows(t, e, `SELECT count(*) FROM system_log WHERE action = 'capture_sync_error'`); n != 1 {
		t.Errorf("capture_sync_error lines after two refused syncs = %d, want 1 per class change", n)
	}

	holdOnlyRole(t, e, e.Rep1, "rep")
	forceDue(t, e, connID)
	if err := registry.SyncOnce(wsCtx, connID); err != nil {
		t.Fatalf("sync after the role came back: %v", err)
	}
	if status, row := readSyncState(t, e, connID); status != "connected" || row.class != nil || provider.pulls != 2 {
		t.Fatalf("after restore: status=%s class=%v pulls=%d, want connected/nil/2", status, row.class, provider.pulls)
	}
}
