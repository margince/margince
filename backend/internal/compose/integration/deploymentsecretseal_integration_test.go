// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The two deployment credentials an earlier release sealed, against a real
// database.
//
// A boot value is never copied into the vault. What needs a database: a
// declared credential seals nothing. A copy sealed before is read once nothing
// declares one, and left in place while one is declared. A vault that cannot
// open what it holds says so, rather than report no license.

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailConfig is a deployment that declares a relay password as a reference,
// loaded with environ as the whole environment. Load refuses a literal secret
// in the file, so a hand-built one would be a shape the product never produces.
func mailConfig(t *testing.T, passwordVar string, environ ...string) *config.Config {
	t.Helper()
	body := "version: 1\nemail:\n  enabled: true\n  from_address: ops@example.test\n  smtp:\n    host: relay.example.test\n    port: 587\n    username: margince\n"
	if passwordVar != "" {
		body += "    password: ${env:" + passwordVar + "}\n"
	}
	return mailSettings(t, body, environ...)
}

// mailSettings loads the relay settings from a margince.yaml body.
func mailSettings(t *testing.T, body string, environ ...string) *config.Config {
	t.Helper()
	cfg, err := configtest.Load(t, body, environ, compose.MailFaults, compose.MailSettings()...)
	if err != nil {
		t.Fatalf("loading the deployment file: %v", err)
	}
	return cfg
}

// relayPassword is the credential the operator mailer would present, read
// through the same constructor the api and the worker build their relay with.
func relayPassword(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, cfg *config.Config, log *slog.Logger) (string, error) {
	relay, err := compose.OperatorMailer(ctx, pool, vault, cfg, log)
	return relay.Password, err
}

// licenseConfig is a deployment that names its license token in a variable,
// loaded with environ as the whole environment.
func licenseConfig(t *testing.T, tokenVar string, environ ...string) *config.Config {
	t.Helper()
	body := "version: 1\n"
	if tokenVar != "" {
		body += "license:\n  token: ${env:" + tokenVar + "}\n"
	}
	cfg, err := configtest.Load(t, body, environ, nil, compose.LicenseToken, compose.LicenseTokenFile)
	if err != nil {
		t.Fatalf("loading the deployment file: %v", err)
	}
	return cfg
}

// sealCtx is the boot's own context: no principal at all, because the seal path
// binds its own system actor. A test that supplied an admin would be proving
// something about a caller the product does not have.
func sealCtx() context.Context { return context.Background() }

// realVault is the LOCAL provider over the harness pool, not keyvault.NewMemory.
//
// The memory provider's GetOn ignores the querier it is handed, and the seal
// path's one in-transaction read — sealedOnTxEquals, the whole mechanism the
// concurrency fix rests on — is exactly a GetOn through a borrowed transaction.
// Against the fake, swapping it back to a pool read would pass every case here
// and deadlock a one-connection pool in production. The root key is fixed and
// meaningless; these tests assert reachability, never key management.
func realVault(t *testing.T, e *Env) keyvault.Vault {
	t.Helper()
	vault, err := keyvault.New(keyvault.Config{RootKey: bytes.Repeat([]byte{9}, 32), Pool: e.Pool})
	if err != nil {
		t.Fatalf("building the test vault: %v", err)
	}
	return vault
}

// differentVault is the same table under a DIFFERENT root key — which is what a
// key rotated or lost in a redeploy looks like from the reading process: the
// ciphertext is all still there and none of it opens.
func differentVault(t *testing.T, e *Env) keyvault.Vault {
	t.Helper()
	vault, err := keyvault.New(keyvault.Config{RootKey: bytes.Repeat([]byte{4}, 32), Pool: e.Pool})
	if err != nil {
		t.Fatalf("building the test vault: %v", err)
	}
	return vault
}

// sealedBlobCount is the evidence for "exactly one copy survives". vault_secret
// carries no workspace_id, and the harness gives each test its own database, so
// a plain count is the honest question.
func sealedBlobCount(t *testing.T, e *Env) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(context.Background(), `SELECT count(*) FROM vault_secret`).Scan(&n); err != nil {
		t.Fatalf("counting sealed blobs: %v", err)
	}
	return n
}

// readCtx reads the ref rows back as an admin. Only the test does this — no
// product surface returns either row — so it exists to assert the ref was
// RECORDED, which a ref held in memory would not be.
func readCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"installation_settings": {Read: true, Update: true},
				"license":               {Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// sealedBefore records secret as an earlier release did: sealed in vault, and
// its ref written through the settings store under the entry's own key.
func sealedBefore(t *testing.T, e *Env, vault keyvault.Vault, entry *settings.Entry[string], secret string) string {
	t.Helper()
	ref, err := vault.Put(sealCtx(), ids.From[ids.WorkspaceKind](e.WS), []byte(secret))
	if err != nil {
		t.Fatalf("sealing the earlier copy: %v", err)
	}
	if err := settings.Set(readCtx(e.WS), compose.NewSettingsStore(e.Pool), entry, string(ref)); err != nil {
		t.Fatalf("recording the earlier copy: %v", err)
	}
	return string(ref)
}

func TestADeclaredRelayPasswordIsReadAndNeverSealed(t *testing.T) {
	e := Setup(t)
	vault := realVault(t, e)
	log := slog.New(slog.DiscardHandler)

	got, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, "SMTP_PASSWORD", "SMTP_PASSWORD=a-relay-password"), log)
	if err != nil {
		t.Fatalf("reading the relay password: %v", err)
	}
	if got != "a-relay-password" {
		t.Fatalf("resolved %q while the deployment declares the password", got)
	}

	ref, err := settings.Get(readCtx(e.WS), compose.NewSettingsStore(e.Pool), identity.SMTPPasswordRef)
	if err != nil {
		t.Fatalf("reading the recorded ref: %v", err)
	}
	if ref != "" || sealedBlobCount(t, e) != 0 {
		t.Fatalf("a declared password was copied into the vault (ref %q, %d blobs)", ref, sealedBlobCount(t, e))
	}

	// The boot after the operator drops the variable: nothing is declared and
	// nothing was sealed, so the relay is unauthenticated.
	after, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, ""), log)
	if err != nil {
		t.Fatalf("reading with nothing declared: %v", err)
	}
	if after != "" {
		t.Errorf("the vault answered %q, want nothing: nothing was sealed", after)
	}
}

// A copy an earlier release sealed is still the answer once nothing declares
// one. The installation that dropped its variable after that release keeps its
// relay.
func TestACopySealedBeforeIsAnsweredOnceTheDeclarationIsGone(t *testing.T) {
	e := Setup(t)
	vault := realVault(t, e)
	log := slog.New(slog.DiscardHandler)
	sealedBefore(t, e, vault, identity.SMTPPasswordRef, "a-relay-password")

	first, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, ""), log)
	if err != nil {
		t.Fatalf("first boot: %v", err)
	}
	second, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, ""), log)
	if err != nil {
		t.Fatalf("second boot: %v", err)
	}
	if first != "a-relay-password" || second != first {
		t.Errorf("the boots answered %q and %q, want the sealed password both times", first, second)
	}
	if got := sealedBlobCount(t, e); got != 1 {
		t.Errorf("%d sealed blobs after two boots read one copy, want 1", got)
	}
}

// A declaration wins over a copy sealed before, and the copy is left in place.
// Destroying it would let one boot with a stale variable or a botched pipeline
// irreversibly destroy a credential it never meant to replace.
func TestADeclarationWinsOverACopySealedBeforeAndLeavesIt(t *testing.T) {
	e := Setup(t)
	vault := realVault(t, e)
	log := slog.New(slog.DiscardHandler)
	superseded := sealedBefore(t, e, vault, identity.SMTPPasswordRef, "the-old-password")

	got, err := relayPassword(sealCtx(), e.Pool, vault, mailConfig(t, "SMTP_PASSWORD", "SMTP_PASSWORD=the-new-password"), log)
	if err != nil {
		t.Fatalf("boot with a declaration: %v", err)
	}
	if got != "the-new-password" {
		t.Errorf("resolved %q, want the declared password over the sealed copy", got)
	}
	ref, err := settings.Get(readCtx(e.WS), compose.NewSettingsStore(e.Pool), identity.SMTPPasswordRef)
	if err != nil {
		t.Fatalf("reading the recorded ref: %v", err)
	}
	if ref != superseded {
		t.Errorf("the declaration repointed the sealed copy from %q to %q; a boot value is never sealed", superseded, ref)
	}
	if _, err := vault.Get(sealCtx(), ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(superseded)); err != nil {
		t.Errorf("the sealed copy is gone from the vault (%v); a declaration must not destroy it", err)
	}
}

// The refusal the sealed copy turns on. Once nothing declares the license, it
// lives in the vault, so a vault that cannot open it must say so. An
// installation told it has no license looks for a token it was already issued,
// and in production that refusal blocks the boot.
func TestAnUnopenableVaultNamesItselfRatherThanReportingNoLicense(t *testing.T) {
	e := Setup(t)
	log := slog.New(slog.DiscardHandler)
	sealedBefore(t, e, realVault(t, e), identity.LicenseTokenRef, "a-license-token")

	// A different vault is what a rotated or dropped root key looks like from
	// here: the ref is recorded and refers to nothing this process can open.
	lost := compose.SealedLicenseTokenSource(sealCtx(), e.Pool, differentVault(t, e), licenseConfig(t, ""), log)
	_, err := lost()
	if err == nil {
		t.Fatal("a vault that cannot open the sealed license reported no license at all")
	}
	for _, want := range []string{"license token", "key vault", "root key"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %q", err, want)
		}
	}
}

// Two roles booting at once with a declared password seal nothing between
// them: no copy to race over, and no ciphertext to strand.
func TestTwoRolesBootingAtOnceSealNothing(t *testing.T) {
	e := Setup(t)
	vault := realVault(t, e)
	log := slog.New(slog.DiscardHandler)
	cfg := mailConfig(t, "SMTP_PASSWORD", "SMTP_PASSWORD=a-relay-password")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	got := make([]string, 2)
	for i := range errs {
		wg.Go(func() {
			got[i], errs[i] = relayPassword(sealCtx(), e.Pool, vault, cfg, log)
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("role %d: %v", i, err)
		}
		if got[i] != "a-relay-password" {
			t.Errorf("role %d resolved %q, want the declared password", i, got[i])
		}
	}
	if n := sealedBlobCount(t, e); n != 0 {
		t.Errorf("%d sealed blobs after two roles read a declared password, want 0", n)
	}
}

// The ledger records that a copy was sealed, never where it went.
//
// A ref is not the secret, but it is the unguessable capability half of the
// handle — which is why the vault strips it from every error it raises and why
// the data reset refuses to name one in a log. audit_log is a log sink like any
// other: admin-readable over /audit-log and exportable. Careful everywhere else
// and verbatim here is careful nowhere.
func TestTheLedgerRecordsTheSealWithoutTheAddress(t *testing.T) {
	e := Setup(t)
	ref := sealedBefore(t, e, realVault(t, e), identity.SMTPPasswordRef, "a-relay-password")
	if ref == "" {
		t.Fatal("nothing was sealed; the assertion below would pass vacuously")
	}

	var rows int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE after::text LIKE '%' || $1 || '%' OR before::text LIKE '%' || $1 || '%'`,
		ref).Scan(&rows); err != nil {
		t.Fatalf("scanning the ledger: %v", err)
	}
	if rows != 0 {
		t.Errorf("%d audit row(s) carry the vault ref verbatim; the ledger records that the seal changed, not its address", rows)
	}
	// And the event itself is still on the record — a redaction that audited
	// nothing would pass the assertion above while losing the row that matters.
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = $1`, identity.SMTPPasswordRef.AuditVerb()); got == 0 {
		t.Error("the seal left no audit row at all; redacting the address must not cost the event")
	}
}
