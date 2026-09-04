// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The keyvault seam end to end: a connector credential is
// sealed in the vault at Connect and resolved from it at Sync, so the
// capture_connection row carries an opaque credential_ref, never the
// credential bytes. Proven on real Postgres with the local (AES-256-GCM)
// provider: the round-trip, cross-workspace ref isolation, a wrong root key
// failing without leaking plaintext, and the additive backfill of a legacy
// auth-bytea row onto the vault.

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// newTestKeyvault builds the local (config-backed) provider over the test
// pool with a fresh random root key. The vault_secret table exists because
// SetupSearch migrated the schema.
func newTestKeyvault(t *testing.T, e *integration.SearchEnv) keyvault.Vault {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	v, err := keyvault.New(keyvault.Config{RootKey: key, Pool: e.Pool})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	return v
}

// authAssertingFake records the Auth it is handed at Sync so a test can prove
// the credential the vault resolved is exactly the one granted. It emits no
// records: this suite is about credential resolution, not capture semantics.
type authAssertingFake struct {
	gotAuth connector.Auth
}

func (f *authAssertingFake) Descriptor() connector.Descriptor {
	return connector.Descriptor{
		// Persisted as capture_connection.provider → must be in the CAP-DDL-2 set.
		Name: "graph", Version: "1.0.0",
		Scopes:   []principal.Scope{principal.ScopeRead},
		RiskTier: mcp.TierAutoExecute,
	}
}

func (f *authAssertingFake) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("granted-token"), nil
}

func (f *authAssertingFake) Sync(_ context.Context, auth connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	f.gotAuth = auth
	return cursor, nil
}

func (f *authAssertingFake) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, connector.ErrSkip
}

func (f *authAssertingFake) HealthCheck(context.Context, connector.Auth) error { return nil }

func TestConnectSealsCredentialInVaultNotOnTheRow(t *testing.T) {
	e := integration.SetupSearch(t)
	vault := newTestKeyvault(t, e)
	registry := newTestCaptureRegistry(e, vault)
	registry.Register(&authAssertingFake{})

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "graph", connector.Auth("granted-token"))
	if err != nil {
		t.Fatal(err)
	}

	var credentialRef *string
	var authBytes []byte
	err = database.WithWorkspaceTx(grantCtx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT credential_ref, auth FROM capture_connection WHERE id = $1`, connID).
			Scan(&credentialRef, &authBytes)
	})
	if err != nil {
		t.Fatal(err)
	}
	if credentialRef == nil || *credentialRef == "" {
		t.Fatal("Connect did not record a credential_ref on the row")
	}
	if authBytes != nil {
		t.Fatalf("Connect left the credential bytes on the row (auth is not NULL): %q", authBytes)
	}
	// The vault holds the sealed credential under the row's ref.
	got, err := vault.Get(context.Background(), ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(*credentialRef))
	if err != nil {
		t.Fatalf("resolving the recorded ref: %v", err)
	}
	if !bytes.Equal(got, []byte("granted-token")) {
		t.Fatalf("vault holds %q, want the granted credential", got)
	}
}

func TestSyncResolvesCredentialFromVault(t *testing.T) {
	e := integration.SetupSearch(t)
	vault := newTestKeyvault(t, e)
	registry := newTestCaptureRegistry(e, vault)
	fake := &authAssertingFake{}
	registry.Register(fake)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "graph", connector.Auth("granted-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.SyncOnce(grantCtx, connID); err != nil {
		t.Fatal(err)
	}
	if string(fake.gotAuth) != "granted-token" {
		t.Fatalf("Sync received %q, want the vault-resolved granted credential", fake.gotAuth)
	}
}

// The local provider on real Postgres must honour the same isolation the
// memory fake does, plus surface a wrong-root-key decrypt as an error (not
// absence) without leaking the plaintext.
func TestLocalVaultIsolationAndWrongKeyOnRealPostgres(t *testing.T) {
	e := integration.SetupSearch(t)
	vault := newTestKeyvault(t, e)
	ctx := context.Background()
	wsA := ids.From[ids.WorkspaceKind](e.WS)
	wsB := ids.New[ids.WorkspaceKind]()

	ref, err := vault.Put(ctx, wsA, []byte("tenant-a-credential"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := vault.Get(ctx, wsA, ref)
	if err != nil || !bytes.Equal(got, []byte("tenant-a-credential")) {
		t.Fatalf("round-trip failed: got %q err %v", got, err)
	}
	if _, err := vault.Get(ctx, wsB, ref); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("cross-workspace Get: got %v, want ErrNotFound", err)
	}

	// A second vault over the SAME table with a DIFFERENT root key finds the
	// row (same ref) but cannot decrypt it — a surfaced error, no plaintext.
	other := newTestKeyvault(t, e)
	_, err = other.Get(ctx, wsA, ref)
	if err == nil {
		t.Fatal("Get under the wrong root key must fail")
	}
	if errors.Is(err, keyvault.ErrNotFound) {
		t.Fatal("a wrong-key decrypt must surface an error, not masquerade as ErrNotFound")
	}
	if bytes.Contains([]byte(err.Error()), []byte("tenant-a-credential")) {
		t.Fatalf("decrypt error leaks the plaintext: %v", err)
	}

	// Delete removes it and is idempotent; Health sees the vault_secret table.
	if err := vault.Delete(ctx, wsA, ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := vault.Get(ctx, wsA, ref); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
	if err := vault.Delete(ctx, wsA, ref); err != nil {
		t.Fatalf("second Delete must be a no-op: %v", err)
	}
	if err := vault.Health(ctx); err != nil {
		t.Fatalf("Health against the migrated schema must pass: %v", err)
	}
}

// systemLogDetails reads the detail payloads this pass filed under one action.
//
// Straight from the table rather than through a reader, because there is no
// reader: system_log is an operator's ledger with no API surface, which is
// exactly why nothing would have noticed the rows being absent.
func systemLogDetails(t *testing.T, e *integration.SearchEnv, action string) []map[string]any {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(),
		`SELECT detail FROM system_log WHERE action = $1 ORDER BY id`, action)
	if err != nil {
		t.Fatalf("reading the %s ledger: %v", action, err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var detail map[string]any
		if err := rows.Scan(&detail); err != nil {
			t.Fatalf("decoding a %s detail: %v", action, err)
		}
		out = append(out, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the %s ledger: %v", action, err)
	}
	return out
}

// Two workers booting at once relocate one credential once, and record it once.
//
// The pass reads its work list in one transaction and claims each row in
// another, so between the two a second boot can take the row — its UPDATE
// carries `AND credential_ref IS NULL` and simply matches nothing. That loser
// must file no ledger row: the relocation is the fact, and a row per boot that
// tried would make an operator scanning for when a credential moved read one
// entry per worker.
//
// The assertion does not depend on WHICH boot wins, which is what keeps this
// from being a coin flip: exactly one relocation is possible, so exactly one
// ledger row is the answer either way.
func TestTwoBootsRelocateOneCredentialOnce(t *testing.T) {
	e := integration.SetupSearch(t)
	vault := newTestKeyvault(t, e)
	first := newTestCaptureRegistry(e, vault)
	first.Register(&authAssertingFake{})
	second := newTestCaptureRegistry(e, vault)
	second.Register(&authAssertingFake{})

	connID := ids.NewV7()
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO capture_connection (id, provider, user_id, scopes, status, auth)
		VALUES ($1, 'graph', $2, $3, 'connected', $4)`,
		connID, e.Rep1, []string{string(principal.ScopeRead)}, []byte("granted-token")); err != nil {
		t.Fatalf("seeding the legacy connection: %v", err)
	}

	var wg sync.WaitGroup
	counts := make([]int, 2)
	errs := make([]error, 2)
	for i, registry := range []*capturemod.Registry{first, second} {
		wg.Add(1)
		go func(slot int, r *capturemod.Registry) {
			defer wg.Done()
			counts[slot], errs[slot] = r.BackfillCredentials(context.Background())
		}(i, registry)
	}
	wg.Wait()

	for slot, err := range errs {
		if err != nil {
			t.Fatalf("boot %d: %v", slot, err)
		}
	}
	// One row, one relocation, however the two interleaved.
	if total := counts[0] + counts[1]; total != 1 {
		t.Errorf("the two boots between them migrated %d rows, want 1 — the claim is "+
			"conditional on credential_ref still being null", total)
	}
	if rows := systemLogDetails(t, e, "capture_credential_relocated"); len(rows) != 1 {
		t.Errorf("the two boots filed %d ledger row(s), want 1 — the boot that lost the "+
			"row moved nothing and has nothing to record", len(rows))
	}
}

func TestBackfillMigratesLegacyAuthRowOntoTheVault(t *testing.T) {
	e := integration.SetupSearch(t)
	vault := newTestKeyvault(t, e)
	registry := newTestCaptureRegistry(e, vault)
	fake := &authAssertingFake{}
	registry.Register(fake)

	// A legacy row: a connected connection whose credential still lives in the
	// auth bytea column, no vault ref yet.
	// Written straight through the owner rather than through e.Seed: that helper
	// binds a workspace, and this table no longer has one to bind.
	connID := ids.NewV7()
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO capture_connection (id, provider, user_id, scopes, status, auth)
		VALUES ($1, 'graph', $2, $3, 'connected', $4)`,
		connID, e.Rep1, []string{string(principal.ScopeRead)}, []byte("granted-token")); err != nil {
		t.Fatalf("seeding the legacy connection: %v", err)
	}

	migrated, err := registry.BackfillCredentials(context.Background())
	if err != nil {
		t.Fatalf("BackfillCredentials: %v", err)
	}
	if migrated != 1 {
		t.Fatalf("backfill migrated %d rows, want 1", migrated)
	}

	// The row now carries a ref and the legacy bytes are cleared.
	var credentialRef *string
	var authBytes []byte
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT credential_ref, auth FROM capture_connection WHERE id = $1`, connID).
			Scan(&credentialRef, &authBytes)
	})
	if err != nil {
		t.Fatal(err)
	}
	if credentialRef == nil || *credentialRef == "" {
		t.Fatal("backfill did not record a credential_ref")
	}
	if authBytes != nil {
		t.Fatalf("backfill left the legacy bytes on the row: %q", authBytes)
	}
	got, err := vault.Get(context.Background(), ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(*credentialRef))
	if err != nil || !bytes.Equal(got, []byte("granted-token")) {
		t.Fatalf("vault does not hold the migrated credential: got %q err %v", got, err)
	}

	// The relocation is in the ledger, and it is the only ledger that can hold
	// it: the row is not pending so no decision lane sees it, and the pass files
	// no audit row on purpose — the credential VALUE did not change, so "who set
	// this credential" is still the connect row's answer. What was missing was
	// WHEN the bytes moved, which is what an operator debugging a connection
	// that broke across a deploy needs (#2552).
	relocations := systemLogDetails(t, e, "capture_credential_relocated")
	if len(relocations) != 1 {
		t.Fatalf("the relocation filed %d system_log row(s), want 1 — the one writer that "+
			"repoints a live connection at new ciphertext left no trace of when", len(relocations))
	}
	if got := relocations[0]["connection_id"]; got != connID.String() {
		t.Errorf("the ledger names connection %v, want %s", got, connID)
	}
	if got := relocations[0]["credential_ref"]; got != *credentialRef {
		t.Errorf("the ledger names ref %v, want the one on the row (%s) — an operator "+
			"correlates the two", got, *credentialRef)
	}
	// And never the material. The bytes are the whole reason this moved into a
	// vault, so a ledger anybody can read must not carry them.
	for key, value := range relocations[0] {
		if text, ok := value.(string); ok && strings.Contains(text, "granted-token") {
			t.Errorf("the ledger's %s carries the credential itself: %q", key, text)
		}
	}

	// A second backfill is a no-op: the row already carries a ref.
	migrated, err = registry.BackfillCredentials(context.Background())
	if err != nil {
		t.Fatalf("second BackfillCredentials: %v", err)
	}
	if migrated != 0 {
		t.Fatalf("idempotent backfill migrated %d rows on the second run, want 0", migrated)
	}
	// And it files nothing either. The pass runs on EVERY boot, so a ledger
	// that recorded each run would count restarts rather than relocations —
	// and an operator scanning for when a credential moved would find one
	// entry per deploy since.
	if again := systemLogDetails(t, e, "capture_credential_relocated"); len(again) != 1 {
		t.Errorf("after a second boot the ledger holds %d row(s), want the one relocation — "+
			"the pass runs every boot and only the move is a fact", len(again))
	}

	// Sync now resolves the migrated credential through the vault.
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if err := registry.SyncOnce(grantCtx, connID); err != nil {
		t.Fatal(err)
	}
	if string(fake.gotAuth) != "granted-token" {
		t.Fatalf("Sync after backfill received %q, want the migrated credential", fake.gotAuth)
	}
}
