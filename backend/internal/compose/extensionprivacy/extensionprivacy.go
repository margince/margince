// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extensionprivacy checks the composed units' personal-data
// declarations at boot and builds the registry the core's privacy paths read.
package extensionprivacy

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// declaredAction is the system_log action recording the digest of a unit's
// declaration against its version.
const declaredAction = "extension.privacy_declared"

// Registry is the declared tables of the given units.
func Registry(exts []extension.Extension) extprivacy.Registry {
	var tables []privacy.Table
	for _, e := range exts {
		tables = append(tables, e.Privacy...)
	}
	return extprivacy.Declared(tables)
}

// Preflight checks one unit's declaration against its own namespace. Whether it
// matches the migrated schema needs the database, so Verify holds that half.
func Preflight(e extension.Extension) error {
	namespace, err := e.Name.Namespace()
	if err != nil {
		return fmt.Errorf("compose: %w", err)
	}
	declared := make(map[string]bool, len(e.Privacy))
	for _, t := range e.Privacy {
		if err := t.Validate(namespace); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if declared[t.Name] {
			return fmt.Errorf("compose: extension %q declares privacy for table %q twice", e.Name, t.Name)
		}
		declared[t.Name] = true
	}
	return nil
}

// verifySchema checks each unit's declaration against its migrated tables.
func verifySchema(ctx context.Context, q extprivacy.Querier, exts []extension.Extension) error {
	for _, e := range exts {
		namespace, err := e.Name.Namespace()
		if err != nil {
			return err
		}
		if err := extprivacy.VerifySchema(ctx, q, namespace, e.Privacy); err != nil {
			return fmt.Errorf("extension %q: %w", e.Name, err)
		}
	}
	return nil
}

// record compares each unit's declaration with what the same version declared
// on an earlier boot of this installation, and records a new version's digest.
// A declaration changes only with the version, which is what an operator reads
// to learn that a unit's data handling changed.
func recordAll(ctx context.Context, tx pgx.Tx, installation string, exts []extension.Extension) error {
	for _, e := range exts {
		if err := record(ctx, tx, installation, e); err != nil {
			return err
		}
	}
	return nil
}

func record(ctx context.Context, tx pgx.Tx, installation string, e extension.Extension) error {
	digest, err := privacy.Digest(e.Privacy)
	if err != nil {
		return err
	}
	var version, last string
	err = tx.QueryRow(ctx, `
		SELECT detail->>'version', detail->>'digest' FROM system_log
		 WHERE action = $1 AND detail->>'installation' = $2 AND detail->>'name' = $3
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		declaredAction, installation, string(e.Name)).Scan(&version, &last)
	switch {
	case err == nil && version == string(e.Version) && last == digest:
		return nil
	case err == nil && version == string(e.Version):
		return fmt.Errorf("extension %q changed its privacy declaration without a new version (still %s); bump the version so the change is visible to operators",
			e.Name, e.Version)
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	_, err = storekit.LogSystem(ctx, tx, declaredAction, map[string]any{
		"name": string(e.Name), "version": string(e.Version), "digest": digest, "installation": installation,
	})
	return err
}

// Ledger is the boot-ledger binding the version check writes through. The root
// compose package owns the installation scope and the advisory lock.
type Ledger struct {
	Scope        func(context.Context) (context.Context, bool, error)
	Lock         string
	Installation func(context.Context) string
}

// Verify checks each unit's declaration against its migrated schema, then
// against what the same version declared on an earlier boot. Before bootstrap
// there is no ledger to compare with, so only the schema check runs.
func Verify(ctx context.Context, pool *pgxpool.Pool, exts []extension.Extension, ledger Ledger) error {
	if err := verifySchema(ctx, pool, exts); err != nil {
		return err
	}
	ctx, bootstrapped, err := ledger.Scope(ctx)
	if err != nil || !bootstrapped {
		return err
	}
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, ledger.Lock, "extension-privacy"); err != nil {
			return fmt.Errorf("compose: serializing the privacy observation: %w", err)
		}
		return recordAll(ctx, tx, ledger.Installation(ctx), exts)
	})
}
