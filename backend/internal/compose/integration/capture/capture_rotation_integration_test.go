// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A provider that replaces the durable credential every time it is used, and
// what the registry does with the replacement.
//
// The unit tests beside the connector prove it REPORTS the rotation. These
// prove the report survives: the vault holds the new secret, the row points at
// it, the old blob is gone, and a lifecycle change that beat the rotation home
// wins.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// rotatingConnector stands in for Microsoft: every sync hands back a
// replacement credential, exactly as the identity platform does on each
// redemption.
type rotatingConnector struct {
	rotations connector.CredentialSink
	// next is the bundle the provider "issues" on the next sync.
	next string
	// duringSync runs at the moment the provider would be answering — the
	// deterministic stand-in for a human disconnecting mid-pull.
	duringSync func()
	// seen is shared BY POINTER with every copy WithCredentialSink makes —
	// which is the whole point of that method, and why a plain counter field
	// reads zero here: the copy is what runs, not the instance the test holds.
	seen *rotationLog
}

// rotationLog is what the sink actually did. Without it a race test passes when
// no rotation was attempted at all, asserting the disconnect's own effect and
// nothing about the fence.
type rotationLog struct {
	attempts int
	lastErr  error
}

func (c *rotatingConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{
		Name: "gmail", Version: "1",
		Scopes:   []principal.Scope{principal.ScopeRead},
		RiskTier: mcp.TierAutoExecute,
		Produces: []datasource.EntityType{datasource.EntityActivity},
	}
}

func (c *rotatingConnector) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("initial-credential"), nil
}

func (c *rotatingConnector) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, nil
}

func (c *rotatingConnector) HealthCheck(context.Context, connector.Auth) error { return nil }

func (c *rotatingConnector) WithCredentialSink(sink connector.CredentialSink) connector.Connector {
	copied := *c
	copied.rotations = sink
	return &copied
}

func (c *rotatingConnector) Sync(ctx context.Context, _ connector.Auth, _ connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	if c.duringSync != nil {
		c.duringSync()
	}
	if c.rotations != nil && c.next != "" {
		c.seen.attempts++
		c.seen.lastErr = c.rotations.Rotated(ctx, connector.Auth(c.next))
		if c.seen.lastErr != nil {
			return nil, c.seen.lastErr
		}
	}
	return connector.Cursor(`{"email":"owner@myco.example"}`), nil
}

// readCredentialRef reads what the connection currently points at.
func readCredentialRef(t *testing.T, e *integration.SearchEnv, connID ids.UUID) string {
	t.Helper()
	var ref *string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT credential_ref FROM capture_connection WHERE id = $1`, connID).Scan(&ref)
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref == nil {
		return ""
	}
	return *ref
}

func TestARotatedCredentialIsSealedAndTheOldOneRetired(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	vault := newTestKeyvault(t, e)
	registry := newTestCaptureRegistry(e, vault)
	fake := &rotatingConnector{next: "replacement-credential", seen: &rotationLog{}}
	registry.Register(fake)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "gmail", connector.Auth("initial-credential"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	before := readCredentialRef(t, e, connID)
	if before == "" {
		t.Fatal("the connection stored no credential ref to rotate")
	}

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := registry.SyncOnce(wsCtx, connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	after := readCredentialRef(t, e, connID)
	if after == before {
		t.Fatal("the connection still points at the credential the provider replaced — it will age out on Microsoft's own schedule")
	}
	sealed, err := vault.Get(wsCtx, ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(after))
	if err != nil {
		t.Fatalf("the row points at a ref the vault cannot resolve: %v", err)
	}
	if string(sealed) != "replacement-credential" {
		t.Fatalf("the sealed credential is %q, want the replacement", sealed)
	}
	// The superseded secret is destroyed, not merely unreferenced: a credential
	// nothing points at but anyone with the ref can still read is exactly what
	// disconnect exists to prevent.
	if _, err := vault.Get(wsCtx, ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(before)); err == nil {
		t.Fatal("the replaced credential is still readable in the vault")
	}
}

// A rotation that arrives after its connection is gone must land nowhere. The
// human's disconnect destroyed the credential; re-pointing the row at a fresh
// blob would resurrect a grant they withdrew.
func TestARotationThatLosesToADisconnectLandsNowhere(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	vault := newTestKeyvault(t, e)
	registry := newTestCaptureRegistry(e, vault)
	fake := &rotatingConnector{next: "replacement-credential", seen: &rotationLog{}}
	registry.Register(fake)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "gmail", connector.Auth("initial-credential"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fake.duringSync = func() {
		if err := registry.Disconnect(grantCtx, "gmail"); err != nil {
			t.Errorf("mid-sync disconnect: %v", err)
		}
	}

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := registry.SyncOnce(wsCtx, connID); err != nil {
		t.Fatalf("SyncOnce over a disconnected connection: %v, want a clean return", err)
	}

	// The fence is only proved if a rotation was actually ATTEMPTED. Without
	// this the assertion below passes on a run where the sink was never bound
	// and Disconnect cleared the ref by itself — which says nothing about
	// whether a rotation racing home would have been fenced out.
	if fake.seen.attempts != 1 {
		t.Fatalf("the connector attempted %d rotation(s), want exactly one to race the disconnect", fake.seen.attempts)
	}
	if fake.seen.lastErr != nil {
		t.Fatalf("the superseded rotation reported %v, want a clean no-op: it lost the race, it did not fail", fake.seen.lastErr)
	}
	if ref := readCredentialRef(t, e, connID); ref != "" {
		t.Fatalf("credential_ref = %q, want none — the disconnect destroyed the credential and the rotation must not restore one", ref)
	}
}

// The other half of the same fence, and the one the generation cannot catch: a
// SAME-ACCOUNT reconnect leaves the generation alone, so only "the credential I
// read is still the credential on the row" tells a rotation of the live grant
// apart from one derived out of a grant that has since been replaced.
func TestARotationThatLosesToAReconnectLandsNowhere(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	vault := newTestKeyvault(t, e)
	registry := newTestCaptureRegistry(e, vault)
	fake := &rotatingConnector{next: "replacement-credential", seen: &rotationLog{}}
	registry.Register(fake)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "gmail", connector.Auth("initial-credential"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fake.duringSync = func() {
		// The same human reconnects the same account mid-pull — a fresh
		// credential on the same row, at the same generation.
		if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("reconnected-credential")); err != nil {
			t.Errorf("mid-sync reconnect: %v", err)
		}
	}

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := registry.SyncOnce(wsCtx, connID); err != nil {
		t.Fatalf("SyncOnce across a reconnect: %v, want a clean return", err)
	}
	if fake.seen.attempts != 1 {
		t.Fatalf("the connector attempted %d rotation(s), want exactly one to race the reconnect", fake.seen.attempts)
	}

	ref := readCredentialRef(t, e, connID)
	sealed, err := vault.Get(wsCtx, ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(ref))
	if err != nil {
		t.Fatalf("resolving the connection's credential: %v", err)
	}
	if string(sealed) != "reconnected-credential" {
		t.Fatalf("the connection holds %q — the stale sync's rotation overwrote the credential the human just reconnected with", sealed)
	}
}
