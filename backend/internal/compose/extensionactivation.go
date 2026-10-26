// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The install step of an extension. Presence under extensions/ still turns a
// unit on. This is what boot does once per unit version: store its settings,
// grant the admin role its objects, and record it as ready. A unit that left the
// composed set has its secrets revoked.
//
// The system_log rows are the only record. GET /extensions reads "ready" from
// the latest of them, so no second table can drift from the boot.

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	extensionActivated = "extension.activated"
	extensionRemoved   = "extension.removed"

	extensionActivationFact = "extension-activation"
)

// ActivatedExtensions answers which unit versions have finished their install
// step: unit name to the version recorded by its latest activation. A unit whose
// latest row is a removal is absent. ctx must carry the workspace.
func ActivatedExtensions(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	var out map[string]string
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		out, err = activatedVersions(ctx, tx)
		return err
	})
	return out, err
}

func activatedVersions(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (detail->>'name') detail->>'name', action, detail->>'version'
		  FROM system_log
		 WHERE action IN ($1, $2) AND detail->>'installation' = $3
		 ORDER BY detail->>'name', occurred_at DESC, id DESC`,
		extensionActivated, extensionRemoved, installationMarker(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, action, version string
		if err := rows.Scan(&name, &action, &version); err != nil {
			return nil, err
		}
		if action == extensionActivated {
			out[name] = version
		}
	}
	return out, rows.Err()
}

// ActivateExtensions installs each composed unit not yet ready at its version.
// It removes each unit that was ready and is no longer composed. A boot with
// nothing new writes nothing. Before bootstrap there is no workspace, so it
// skips.
//
// The grants and the revocation commit before the ledger row and are safe to
// repeat. A boot that dies half way finishes on the next one.
func ActivateExtensions(
	ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, log *slog.Logger,
	exts []extension.Extension, verbs []extension.Verb,
) error {
	ctx, bootstrapped, err := bootLedgerScope(ctx, pool, "system:extension-activation")
	if err != nil || !bootstrapped {
		return err
	}
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, bootLedgerLock, extensionActivationFact); err != nil {
			return fmt.Errorf("compose: serializing the extension activation: %w", err)
		}
		done, err := activatedVersions(ctx, tx)
		if err != nil {
			return err
		}
		for _, e := range exts {
			if done[string(e.Name)] == string(e.Version) {
				continue
			}
			if err := activateExtension(ctx, pool, tx, log, e, unitRbacObjects(e.Name, verbs)); err != nil {
				return fmt.Errorf("compose: activating extension %q: %w", e.Name, err)
			}
		}
		for name, version := range done {
			if slices.ContainsFunc(exts, func(e extension.Extension) bool { return string(e.Name) == name }) {
				continue
			}
			if err := removeExtension(ctx, pool, vault, tx, log, name, version); err != nil {
				return fmt.Errorf("compose: removing extension %q: %w", name, err)
			}
		}
		return nil
	})
}

func activateExtension(
	ctx context.Context, pool *pgxpool.Pool, tx pgx.Tx, log *slog.Logger, e extension.Extension, objects []string,
) error {
	granted, err := identity.NewService(pool).GrantExtensionObjects(ctx, objects)
	if err != nil {
		return err
	}
	reports, err := settings.SeedExtensions(ctx, tx, []extension.Extension{e})
	if err != nil {
		return err
	}
	stored := 0
	for _, r := range reports {
		stored += len(r.Stored)
	}
	if _, err := storekit.LogSystem(ctx, tx, extensionActivated, map[string]any{
		unitKey: string(e.Name), versionKey: string(e.Version), "rights_granted": granted,
		"settings_stored": stored, installationKey: installationMarker(ctx),
	}); err != nil {
		return err
	}
	log.Info("extension activated", "unit", e.Name, versionKey, e.Version, "rights_granted", len(granted))
	return nil
}

func removeExtension(
	ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, tx pgx.Tx, log *slog.Logger, name, version string,
) error {
	revoked, err := extsecrets.RevokeUnit(ctx, pool, vault, name)
	if err != nil {
		return err
	}
	if _, err := storekit.LogSystem(ctx, tx, extensionRemoved, map[string]any{
		unitKey: name, versionKey: version, "secrets_revoked": revoked, installationKey: installationMarker(ctx),
	}); err != nil {
		return err
	}
	log.Info("extension removed", "unit", name, "secrets_revoked", revoked)
	return nil
}
