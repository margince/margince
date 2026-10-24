// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

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

// extensionPrivacyDeclared is the system_log action recording the digest of a
// unit's privacy declaration against its version.
const extensionPrivacyDeclared = "extension.privacy_declared"

const extensionPrivacyFact = "extension-privacy"

// The detail keys an observation is filtered by.
const (
	installationKey = "installation"
	unitKey         = "name"
	versionKey      = "version"
)

// ExtensionData is the registry of this boot's composed units' declared tables.
// Every eraser, export and merge store built outside a test is handed it.
func ExtensionData() extprivacy.Registry { return ExtensionPrivacy(ComposedExtensions()) }

// ExtensionPrivacy is the registry of the given units' declared tables.
func ExtensionPrivacy(exts []extension.Extension) extprivacy.Registry {
	var tables []privacy.Table
	for _, e := range exts {
		tables = append(tables, e.Privacy...)
	}
	return extprivacy.New(tables)
}

// VerifyExtensionPrivacy checks each unit's declaration against its migrated
// schema, and against what the same version declared on an earlier boot.
//
// A declaration changes only with the unit's version, which is what an operator
// reads to learn that a unit's data handling changed. Before bootstrap there is
// no ledger to compare with, so only the schema check runs.
func VerifyExtensionPrivacy(ctx context.Context, pool *pgxpool.Pool, exts []extension.Extension) error {
	for _, e := range exts {
		namespace, err := e.Name.Namespace()
		if err != nil {
			return err
		}
		if err := extprivacy.VerifySchema(ctx, pool, namespace, e.Privacy); err != nil {
			return fmt.Errorf("extension %q: %w", e.Name, err)
		}
	}
	ctx, bootstrapped, err := bootLedgerScope(ctx, pool, "system:extension-privacy")
	if err != nil || !bootstrapped {
		return err
	}
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, bootLedgerLock, extensionPrivacyFact); err != nil {
			return fmt.Errorf("compose: serializing the privacy observation: %w", err)
		}
		for _, e := range exts {
			if err := recordPrivacyDeclaration(ctx, tx, e); err != nil {
				return err
			}
		}
		return nil
	})
}

func recordPrivacyDeclaration(ctx context.Context, tx pgx.Tx, e extension.Extension) error {
	digest, err := privacy.Digest(e.Privacy)
	if err != nil {
		return err
	}
	var version, last string
	err = tx.QueryRow(ctx, `
		SELECT detail->>'version', detail->>'digest' FROM system_log
		 WHERE action = $1 AND detail->>'installation' = $2 AND detail->>'name' = $3
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		extensionPrivacyDeclared, installationMarker(ctx), string(e.Name)).Scan(&version, &last)
	switch {
	case err == nil && version == string(e.Version) && last == digest:
		return nil
	case err == nil && version == string(e.Version):
		return fmt.Errorf("extension %q changed its privacy declaration without a new version (still %s); bump the version so the change is visible to operators",
			e.Name, e.Version)
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	_, err = storekit.LogSystem(ctx, tx, extensionPrivacyDeclared, map[string]any{
		unitKey: string(e.Name), versionKey: string(e.Version), "digest": digest, installationKey: installationMarker(ctx),
	})
	return err
}
