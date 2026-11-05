// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The copies of two deployment credentials an earlier release sealed in the
// key vault: the outbound-relay password and the license token. A declared
// value wins and is never sealed. A copy is read once nothing declares the
// credential, and `password: ${none}` deletes the relay's.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deploymentSecret is one credential an earlier release sealed: where its ref
// is recorded, and what to call it when something goes wrong.
type deploymentSecret struct {
	// ref is the settings entry holding the vault ref.
	ref *settings.Entry[string]
	// name is the credential in an operator's words, for the boot log and for
	// the refusal. "the license token", not "license_token_ref".
	name string
	// declaredAt is where the deployment spells the credential, so a refusal
	// names the line to edit rather than the row to inspect.
	declaredAt string
}

// vaultBinding is what a seal needs from the process: somewhere to put a blob,
// somewhere to record the ref, and the workspace both are scoped to. Grouped
// because every function below needs all three and none of them means anything
// alone.
type vaultBinding struct {
	pool  *pgxpool.Pool
	vault keyvault.Vault
	ws    ids.WorkspaceID
	log   *slog.Logger
}

// resolveSecret returns the credential this process should use: the declared
// one, else the copy an earlier release sealed. A declared credential always
// wins, so a boot never fails on the vault while the operator supplies the
// value themselves.
func resolveSecret(ctx context.Context, b vaultBinding, s deploymentSecret, declared string) (string, error) {
	if declared != "" {
		return declared, nil
	}
	stored, err := settings.Get(ctx, NewSettingsStore(b.pool), s.ref)
	if err != nil {
		return "", fmt.Errorf("compose: reading where %s is sealed: %w", s.name, err)
	}
	if stored == "" {
		return "", nil
	}
	return b.open(ctx, s, stored)
}

// open reads a sealed credential, and says so in the operator's terms when it
// cannot.
//
// This refusal is the whole reason the sealed copy is safe to depend on. Once
// the declaration is gone the vault holds the only copy, so a vault this process
// cannot reach is not "no license configured" or "no relay password" — it is a
// credential that exists and is out of reach, which is a different problem with
// a different fix. Reporting it as absence would send an operator looking for a
// token they were already issued.
func (b vaultBinding) open(ctx context.Context, s deploymentSecret, stored string) (string, error) {
	secret, err := b.vault.Get(ctx, b.ws, keyvault.Ref(stored))
	if err != nil {
		return "", fmt.Errorf("compose: %s is sealed in the key vault and cannot be opened: %w — "+
			"check the vault root key is the one this installation sealed with", s.name, err)
	}
	if len(secret) == 0 {
		return "", fmt.Errorf("compose: %s is sealed in the key vault but the sealed value is empty — "+
			"declare the credential at %s", s.name, s.declaredAt)
	}
	return string(secret), nil
}

// secretSealActor names the boot in the audit trail when a sealed copy's ref
// row is read or removed. Distinct from the routing seed's actor, so the audit
// rows of the two tell apart.
const secretSealActor = "deployment-secret-seal"

// smtpPassword and licenseToken are the two credentials that take this path.
var (
	smtpPassword = deploymentSecret{
		ref:        identity.SMTPPasswordRef,
		name:       "the outbound-mail password",
		declaredAt: "email.smtp.password",
	}
	licenseToken = deploymentSecret{
		ref:        identity.LicenseTokenRef,
		name:       "the license token",
		declaredAt: "license.token",
	}
)

// sealedSMTPPassword resolves the relay credential the mailer authenticates
// with: the declared one, else a sealed copy. A password declared absent
// resolves to nothing; RemoveDeclaredAbsentCredentials deleted its sealed copy
// at boot.
//
// An empty result is an unauthenticated relay, which is a posture rather than a
// mistake: it is what an installation that names no password has always had.
func sealedSMTPPassword(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, cfg *config.Config, log *slog.Logger) (string, error) {
	if smtpPasswordRemoved(cfg) {
		return "", nil
	}
	declared, err := declaredSMTPPassword(cfg)
	if err != nil {
		return "", err
	}
	return sealedSecret(ctx, pool, vault, smtpPassword, declared, log)
}

// SealedLicenseTokenSource is the license watcher's token source, reading the
// vault where the deployment no longer declares one.
//
// A SOURCE rather than a token, because the watcher re-reads it. Only a
// license.token_file renewed in place takes effect on the next re-check;
// license.token is fixed at boot.
func SealedLicenseTokenSource(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, cfg *config.Config, log *slog.Logger) func() (string, error) {
	declared := declaredLicenseToken(cfg)
	return func() (string, error) {
		token, err := declared()
		if err != nil {
			return "", err
		}
		return sealedSecret(ctx, pool, vault, licenseToken, token, log)
	}
}

// sealedSecret binds the boot principal and hands off to resolveSecret.
//
// An unprovisioned installation (ADR-0105: claimed but not yet bootstrapped)
// has no workspace, so nothing can have been sealed for it. What the
// deployment declares is the whole answer. That is not an error: every tenant
// route answers 503 until the claim runs, and a refusal here would block it.
//
// singletonWorkspace documents EnsureInstallation as having already refused a
// multi-workspace database, and on the WORKER that is the api's doing rather
// than this process's — the worker never calls it. ADR-0061 makes a second
// workspace unreachable, so the precondition holds in fact; it is worth saying
// that it holds for a reason outside this call rather than because of one.
func sealedSecret(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, s deploymentSecret, declared string, log *slog.Logger) (string, error) {
	if vault == nil {
		// No vault, which — because keyvault.FromEnv refuses a boot that has
		// sealed secrets and no root key — also means this installation has
		// sealed nothing. So there is no ref to find, the declaration is the
		// whole answer, and saying so costs no database read.
		//
		// That refusal is what makes this safe, rather than an assumption made
		// here: without it a root key dropped in a redeploy would arrive at this
		// line with a recorded ref nobody looked for, and a production boot
		// would call an installation that has a license unlicensed.
		return declared, nil
	}
	ws, err := singletonWorkspace(ctx, pool)
	if err != nil {
		return "", err
	}
	if ws == (ids.UUID{}) {
		return declared, nil
	}
	b := vaultBinding{pool: pool, vault: vault, ws: ids.From[ids.WorkspaceKind](ws), log: log}
	return resolveSecret(bootCtx(ctx, ws, secretSealActor), b, s, declared)
}

// RemoveDeclaredAbsentCredentials deletes the sealed copy of every deployment
// credential the deployment declares absent. Both serving roles call it at
// boot, whether or not outbound mail is enabled. A removal is a revocation: a
// leaked password must not stay sealed for the day mail is turned back on.
func RemoveDeclaredAbsentCredentials(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, cfg *config.Config, log *slog.Logger) error {
	if !smtpPasswordRemoved(cfg) {
		return nil
	}
	return unsealSecret(ctx, pool, vault, smtpPassword, log)
}

// unsealSecret binds the boot principal and hands off to unseal. A process
// with no vault, or an installation with no workspace yet, has sealed nothing,
// so there is nothing to remove.
func unsealSecret(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, s deploymentSecret, log *slog.Logger) error {
	if vault == nil {
		return nil
	}
	ws, err := singletonWorkspace(ctx, pool)
	if err != nil {
		return err
	}
	if ws == (ids.UUID{}) {
		return nil
	}
	b := vaultBinding{pool: pool, vault: vault, ws: ids.From[ids.WorkspaceKind](ws), log: log}
	return b.unseal(bootCtx(ctx, ws, secretSealActor), s)
}

// unseal deletes the sealed copy of a credential declared absent: the blob
// first, then the row that names it. In that order a failed step leaves the
// row, so the next boot retries, and a vault delete is idempotent. The other
// order could strand the ciphertext with nothing pointing at it. Failures are
// logged, not returned: this process runs on no credential either way.
func (b vaultBinding) unseal(ctx context.Context, s deploymentSecret) error {
	store := NewSettingsStore(b.pool)
	stored, err := settings.Get(ctx, store, s.ref)
	if err != nil {
		return fmt.Errorf("compose: reading where %s is sealed: %w", s.name, err)
	}
	if stored == "" {
		return nil
	}
	if err := b.vault.Delete(ctx, b.ws, keyvault.Ref(stored)); err != nil {
		b.log.ErrorContext(ctx, "cannot delete a sealed deployment credential the deployment declares removed; it is not used, and the next boot tries again",
			"credential_name", s.name, "declared_at", s.declaredAt, "error", err)
		return nil
	}
	var removed bool
	err = store.WriteTx(ctx, func(tx pgx.Tx) error {
		// The lock record() takes, so a seal cannot repoint the row between
		// this comparison and the delete below.
		if err := settings.LockForWrite(ctx, tx, s.ref.Key()); err != nil {
			return err
		}
		current, err := settings.GetTx(ctx, tx, s.ref)
		if err != nil {
			return err
		}
		if current != stored {
			// A ref other than the one just deleted was recorded meanwhile;
			// it is not this boot's to remove.
			return nil
		}
		removed, err = settings.DeleteTx(ctx, store, tx, s.ref)
		return err
	})
	if err != nil {
		b.log.ErrorContext(ctx, "deleted a sealed deployment credential but not the record of where it was; the next boot tries again",
			"credential_name", s.name, "declared_at", s.declaredAt, "error", err)
		return nil
	}
	if removed {
		b.log.InfoContext(ctx, "removed a sealed deployment credential on this boot; the deployment declares it absent",
			"credential_name", s.name, "declared_at", s.declaredAt)
	}
	return nil
}

// deploymentSecretRefKeys is the settings key each deployment credential keeps
// its vault ref under.
//
// Read by the reset's orphan census, which purges every sealed ref nothing
// references — and these two are referenced from a SETTINGS row rather than
// from a column named `*_ref`, so a census built from the schema alone cannot
// see them and would purge the licence off a running installation.
//
// Derived from the declarations above rather than listed again, so a third
// deployment credential is covered by being declared.
func deploymentSecretRefKeys() []string {
	secrets := []deploymentSecret{smtpPassword, licenseToken}
	keys := make([]string, 0, len(secrets))
	for _, s := range secrets {
		keys = append(keys, s.ref.Key())
	}
	return keys
}
