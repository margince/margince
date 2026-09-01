// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The write path for the installation's Google OAuth app: an admin puts a pair
// in, the connect transport resolves it, and nothing along the way can read the
// secret back.
//
// Against a REAL vault and a real settings store, because what is being proved
// is that the ciphertext lands somewhere the resolver can reach — a fake vault
// would prove the store calls Put and nothing about whether the app works.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// testClientID is a client id Google would issue. The suffix is load-bearing:
// the store refuses anything else, so a fixture without it would exercise the
// refusal rather than the write.
const testClientID = "1234567890-abcdefghijklmnop.apps.googleusercontent.com"

// captureCtx binds a human holding exactly the given capture_settings grant.
func (e *SearchEnv) captureCtx(grant principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{capture.SettingsObject: grant},
			RowScope: principal.RowScopeAll,
		},
	})
}

func (e *SearchEnv) captureAdmin() context.Context {
	return e.captureCtx(principal.ObjectGrant{Read: true, Update: true})
}

func googleAppStore(t *testing.T, e *SearchEnv) *capture.ConnectorAppStore {
	t.Helper()
	return capture.NewConnectorAppStore(compose.NewSettingsStore(e.Pool), searchVault(t, e), discard())
}

// The stored setting holds a REFERENCE and never the secret.
//
// Asserted over the row's raw bytes rather than through the store's own reader,
// because the reader is where a redaction would live and the row is what an
// operator with database access actually sees.
func TestTheStoredGoogleAppHoldsARefAndNeverTheSecret(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	const secret = "GOCSPX-super-secret-value"

	if err := googleAppStore(t, e).Set(ctx, capture.AppProviderGoogle, testClientID, secret, ""); err != nil {
		t.Fatalf("storing the app: %v", err)
	}

	raw := storedGoogleAppJSON(t, e)
	if strings.Contains(raw, secret) {
		t.Errorf("the setting row carries the client secret verbatim: %s", raw)
	}
	// The client id IS there, deliberately — it is not a secret, and an operator
	// needs to see which app the installation uses.
	if !strings.Contains(raw, testClientID) {
		t.Errorf("the setting row does not record the client id: %s", raw)
	}
}

// The audit trail records that the app changed and never what it points at.
//
// `audit_log` is a log sink like any other — admin-readable over /audit-log and
// exportable — so a vault ref written verbatim into a before/after image is a
// capability handle sitting in the one place an installation hands out
// wholesale. `settings.Entry.AsSecretReference` is what redacts it.
//
// Asserted against the ROW rather than the entry's declaration, because what
// matters is the bytes that land in the sink: a redaction that stops being
// applied leaves the declaration looking correct.
func TestTheAuditTrailOfAGoogleAppChangeCarriesNoRef(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()

	if err := googleAppStore(t, e).Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-audited", ""); err != nil {
		t.Fatalf("storing: %v", err)
	}
	ref := storedGoogleRef(ctx, t, e)
	if ref == "" {
		t.Fatal("nothing was stored, so the test would pass on an empty trail")
	}

	images := googleAppAuditImages(t, e)
	if images == "" {
		t.Fatal("the app change wrote no audit row at all")
	}
	if strings.Contains(images, ref) {
		t.Errorf("the audit trail carries the vault ref verbatim: %s", images)
	}
	// Not only the whole ref — its unguessable tail is the capability half, and a
	// partial write would be just as reachable.
	if tail := ref[len(ref)-12:]; strings.Contains(images, tail) {
		t.Errorf("the audit trail carries a fragment of the vault ref: %s", images)
	}
}

// The connect transport resolves what the admin stored.
//
// Through the store's own Credentials, which is what compose hands the
// transport — a test that read the setting and unsealed by hand would prove the
// vault works and nothing about whether the app reaches the flow that needs it.
func TestTheStoredGoogleAppIsWhatTheConnectTransportResolves(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	store := googleAppStore(t, e)

	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-first", ""); err != nil {
		t.Fatalf("storing the app: %v", err)
	}
	app, ok, err := store.Credentials(ctx, capture.AppProviderGoogle)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if !ok {
		t.Fatal("a stored app did not resolve")
	}
	if app.ClientID != testClientID || app.ClientSecretRef != "GOCSPX-first" {
		t.Errorf("resolved %q / %q, want the stored pair", app.ClientID, app.ClientSecretRef)
	}
}

// An installation with no app resolves nothing, and that is not an error: the
// connect surface says so rather than failing.
func TestNoGoogleAppResolvesToNothingRatherThanAnError(t *testing.T) {
	e := SetupSearch(t)

	_, ok, err := googleAppStore(t, e).Credentials(e.captureAdmin(), capture.AppProviderGoogle)
	if err != nil {
		t.Fatalf("resolving an unconfigured installation: %v", err)
	}
	if ok {
		t.Error("an installation with no app resolved one")
	}
}

// Rotating replaces the secret and destroys the superseded one.
//
// The old blob is GONE, not merely unreferenced: a credential no ref names is
// one no operator can find to delete, and it stays decryptable by anything
// holding the root key.
func TestRotatingTheGoogleAppRetiresTheOldSecret(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	store := googleAppStore(t, e)

	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-first", ""); err != nil {
		t.Fatalf("storing: %v", err)
	}
	firstRef := storedGoogleRef(ctx, t, e)

	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-second", ""); err != nil {
		t.Fatalf("rotating: %v", err)
	}
	if storedGoogleRef(ctx, t, e) == firstRef {
		t.Fatal("the ref did not move, so the rotation stored nothing new")
	}
	app, _, err := store.Credentials(ctx, capture.AppProviderGoogle)
	if err != nil {
		t.Fatalf("resolving after rotation: %v", err)
	}
	if app.ClientSecretRef != "GOCSPX-second" {
		t.Errorf("resolved %q after rotation, want the new secret", app.ClientSecretRef)
	}
	assertVaultRefGone(ctx, t, e, firstRef, "the superseded client secret")
}

// A CLIENT ID CANNOT BE REPLACED WHILE MAILBOXES ARE CONNECTED UNDER IT.
//
// A refresh token belongs to the client that issued it. Swapping the id makes
// every stored token unrefreshable, and the vendor answers `invalid_client` — so
// the mailboxes stop syncing one by one, at whatever hour their next refresh
// falls, and from the inside it looks like every mailbox revoking access at
// once.
//
// The SECRET stays rotatable against the same id, which is the whole point of
// being able to rotate one.
func TestReplacingTheClientIDIsRefusedWhileConnectionsHoldTokensFromTheOldOne(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	store := googleAppStore(t, e)

	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-first", ""); err != nil {
		t.Fatalf("storing: %v", err)
	}

	// A ROTATION FIRST, before any connection exists, so the refusal below is
	// about the id and not about the write path being closed.
	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-second", ""); err != nil {
		t.Fatalf("rotating the secret with no connections: %v", err)
	}

	seedGmailConnection(ctx, t, e)

	// The secret still rotates with a mailbox connected — same client, same
	// tokens.
	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-third", ""); err != nil {
		t.Fatalf("rotating the secret with a connection: %v", err)
	}

	const otherClientID = "9999999999-zyxwvutsrqponmlk.apps.googleusercontent.com"
	err := store.Set(ctx, capture.AppProviderGoogle, otherClientID, "GOCSPX-fourth", "")
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) {
		t.Fatalf("replacing the client id answered %v, want a refusal — every connected mailbox holds a "+
			"refresh token the new client cannot use", err)
	}
	if !strings.Contains(invalid.Reason, "refresh token") {
		t.Errorf("the refusal reads %q, which does not tell the operator why", invalid.Reason)
	}

	// And the stored app is untouched: a refused write that half-committed
	// would be worse than the change it refused.
	app, _, err := store.Credentials(ctx, capture.AppProviderGoogle)
	if err != nil {
		t.Fatalf("resolving after the refusal: %v", err)
	}
	if app.ClientID != testClientID {
		t.Errorf("client id = %q after a refused change, want the one that was there", app.ClientID)
	}
	if app.ClientSecretRef != "GOCSPX-third" {
		t.Errorf("secret = %q after a refused change, want the one that was there", app.ClientSecretRef)
	}

	// A DISCONNECTED mailbox strands nobody, so the change goes through once
	// the operator has done what the refusal asked.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET status = 'disconnected' WHERE provider = 'gmail'`)
		return err
	}); err != nil {
		t.Fatalf("disconnecting: %v", err)
	}
	if err := store.Set(ctx, capture.AppProviderGoogle, otherClientID, "GOCSPX-fifth", ""); err != nil {
		t.Fatalf("replacing the client id with nothing connected: %v — the refusal has no way out", err)
	}
}

// seedGmailConnection writes one connected gmail mailbox. Written directly
// because the subject is the ROW the app change would strand, not how a consent
// callback puts it there.
func seedGmailConnection(ctx context.Context, t *testing.T, e *SearchEnv) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_connection (provider, user_id, scopes, status, auth)
			VALUES ('gmail', $1, '{}', 'connected', $2)`,
			e.Rep1, []byte(`{"refresh_token":"r","granted":[]}`))
		return err
	}); err != nil {
		t.Fatalf("seeding the gmail connection: %v", err)
	}
}

// Removing clears the app and destroys the secret, and removing an absent one
// succeeds: the caller asked for a state and that state already holds.
func TestRemovingTheGoogleAppIsIdempotentAndDestroysTheSecret(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	store := googleAppStore(t, e)

	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-only", ""); err != nil {
		t.Fatalf("storing: %v", err)
	}
	ref := storedGoogleRef(ctx, t, e)

	if err := store.Remove(ctx, capture.AppProviderGoogle); err != nil {
		t.Fatalf("removing: %v", err)
	}
	status, err := store.Read(ctx, capture.AppProviderGoogle)
	if err != nil {
		t.Fatal(err)
	}
	if status.Configured || status.ClientID != "" {
		t.Errorf("after removal the app reads %+v, want empty", status)
	}
	assertVaultRefGone(ctx, t, e, ref, "the removed client secret")
	if err := store.Remove(ctx, capture.AppProviderGoogle); err != nil {
		t.Errorf("removing an absent app failed: %v", err)
	}
}

// A client id that is not one is refused BEFORE a secret is sealed for it.
//
// The order matters: sealing first would leave a blob to retire on every typo.
// And the refusal has to name the mistake — a value copied off the same console
// screen that is not the client id is almost always the project number or an API
// key, which would otherwise surface much later as an opaque invalid_client from
// Google on somebody's first connect.
func TestAMalformedClientIDIsRefusedWithoutSealingAnything(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()

	before := googleSealedCount(t, e)
	err := googleAppStore(t, e).Set(ctx, capture.AppProviderGoogle, "1234567890", "GOCSPX-never-sealed", "")
	if err == nil {
		t.Fatal("a project number was accepted as a client id")
	}
	if !strings.Contains(err.Error(), "apps.googleusercontent.com") {
		t.Errorf("the refusal does not name what a client id looks like: %v", err)
	}
	if after := googleSealedCount(t, e); after != before {
		t.Errorf("sealed blobs went from %d to %d; a refused write sealed something", before, after)
	}
}

// A seat that may not change capture settings cannot store an app, and nothing
// is sealed on the way to the refusal.
//
// Counted rather than asserted on the error alone: a gate that refuses AFTER
// sealing leaves a credential in the vault that no ref names, which is the
// failure an error-only assertion cannot see.
func TestAnUnprivilegedSeatCannotStoreAGoogleApp(t *testing.T) {
	e := SetupSearch(t)
	before := googleSealedCount(t, e)

	err := googleAppStore(t, e).Set(e.captureCtx(principal.ObjectGrant{Read: true}), capture.AppProviderGoogle, testClientID, "GOCSPX-refused", "")
	if err == nil {
		t.Fatal("a seat holding only read stored the installation's Google app")
	}
	if after := googleSealedCount(t, e); after != before {
		t.Errorf("sealed blobs went from %d to %d; the refusal came after sealing", before, after)
	}
}

// storedGoogleRef reads the ref the setting currently records.
func storedGoogleRef(ctx context.Context, t *testing.T, e *SearchEnv) string {
	t.Helper()
	app, err := settings.Get(ctx, compose.NewSettingsStore(e.Pool), capture.GoogleAppSetting)
	if err != nil {
		t.Fatalf("reading the stored app: %v", err)
	}
	return app.ClientSecretRef
}

// storedGoogleAppJSON returns the setting row's raw bytes — what somebody with
// database access sees, rather than what a reader chooses to show them.
func storedGoogleAppJSON(t *testing.T, e *SearchEnv) string {
	t.Helper()
	var raw string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(value::text, '') FROM setting WHERE key = $1`, capture.GoogleAppKey).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the setting row: %v", err)
	}
	return raw
}

// googleAppAuditImages returns every before/after image the app's writes wrote.
func googleAppAuditImages(t *testing.T, e *SearchEnv) string {
	t.Helper()
	var images string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT coalesce(string_agg(coalesce(before::text, '') || '|' || coalesce(after::text, ''), ' '), '')
			FROM audit_log WHERE entity_type = $1`, capture.SettingsObject).Scan(&images)
	}); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	return images
}

// googleSealedCount counts the vault's blobs, so a case can assert that a
// refused write sealed NOTHING.
func googleSealedCount(t *testing.T, e *SearchEnv) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM vault_secret`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting sealed blobs: %v", err)
	}
	return n
}

// assertVaultRefGone fails unless the ref is gone for the ONE reason that means
// it was retired: keyvault.ErrNotFound.
//
// `err != nil` is not the same assertion and is the weaker one by a long way. A
// pool exhaustion, a revoked grant or a vault whose root key moved all return
// non-nil too, and each would let this pass while the secret sat there readable
// — the test reporting the cleanup it exists to prove without any cleanup
// having happened.
func assertVaultRefGone(ctx context.Context, t *testing.T, e *SearchEnv, ref, what string) {
	t.Helper()
	_, err := searchVault(t, e).Get(ctx, ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(ref))
	switch {
	case err == nil:
		t.Errorf("%s is still readable from the vault", what)
	case !errors.Is(err, keyvault.ErrNotFound):
		t.Errorf("%s: reading the retired ref gave %v, want keyvault.ErrNotFound — "+
			"a different failure hides whether the secret was destroyed", what, err)
	}
}

// Credentials is the resolve-for-USE path — what the connect transport calls to
// exchange an authorization code. Separate from Read because the two answer
// different questions: Read tells a person what is configured, this unseals the
// secret and hands the server what it needs to talk to Google.
func TestGoogleAppCredentialsResolveTheSealedSecret(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	store := googleAppStore(t, e)

	// An installation with no app is not an error: the connect surface says so
	// rather than failing, so ok=false must come back with a nil error.
	if _, ok, err := store.Credentials(ctx, capture.AppProviderGoogle); err != nil || ok {
		t.Fatalf("Credentials on an unconfigured installation = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-resolve-me", ""); err != nil {
		t.Fatalf("storing: %v", err)
	}
	app, ok, err := store.Credentials(ctx, capture.AppProviderGoogle)
	if err != nil || !ok {
		t.Fatalf("Credentials after storing = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if app.ClientID != testClientID || app.ClientSecretRef != "GOCSPX-resolve-me" {
		t.Fatalf("Credentials resolved (%q, %q), want the stored pair", app.ClientID, app.ClientSecretRef)
	}

	// Removed, it stops resolving — the transport must not keep serving an app
	// the operator has taken away.
	if err := store.Remove(ctx, capture.AppProviderGoogle); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if _, ok, err := store.Credentials(ctx, capture.AppProviderGoogle); err != nil || ok {
		t.Fatalf("Credentials after removal = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}

// The ceiling exists so a caller cannot seal a megabyte and have it read back on
// every request. The contract declares maxLength on both halves and nothing
// generated enforces it: oapi-codegen emits no validation, and the decoder caps
// only the whole body.
func TestGoogleAppRefusesAnOversizedHalfWithoutSealingIt(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	store := googleAppStore(t, e)

	oversized := strings.Repeat("x", 513)
	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, oversized, ""); err == nil {
		t.Fatal("an oversized client secret was accepted")
	}
	if err := store.Set(ctx, capture.AppProviderGoogle, oversized+".apps.googleusercontent.com", "GOCSPX-fine", ""); err == nil {
		t.Fatal("an oversized client id was accepted")
	}
	// Refused BEFORE anything was sealed: the app still reads as empty, so no
	// blob was written that would then need retiring.
	status, err := store.Read(ctx, capture.AppProviderGoogle)
	if err != nil {
		t.Fatal(err)
	}
	if status.Configured {
		t.Fatalf("a refused oversized write left the app reading %+v, want empty", status)
	}
}
