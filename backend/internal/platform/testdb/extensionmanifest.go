// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

const extensionManifestEnv = "MARGINCE_TEST_EXTENSION_MANIFEST"

func testExtensionNamespaces() ([]dbmigrate.Namespace, error) {
	path := os.Getenv(extensionManifestEnv)
	if path == "" {
		return nil, errors.New("the composed test migration manifest is missing; run make test-it or make test-integration from this worktree")
	}
	body, err := os.ReadFile(path) // #nosec G304 G703 -- the local test runner supplies this generated manifest, never a request
	if err != nil {
		return nil, fmt.Errorf("reading the composed test migration manifest: %w", err)
	}
	var namespaces []dbmigrate.Namespace
	if err := json.Unmarshal(body, &namespaces); err != nil {
		return nil, fmt.Errorf("decoding the composed test migration manifest: %w", err)
	}
	return namespaces, nil
}

func unlistedExtensionLedger(ctx context.Context, owner *pgx.Conn, namespaces []dbmigrate.Namespace) (string, error) {
	expected := map[string]bool{}
	for _, namespace := range namespaces {
		expected["schema_migrations_"+namespace.Name] = true
	}
	rows, err := owner.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND starts_with(tablename,'schema_migrations_ext_')`)
	if err != nil {
		return "", err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	for _, name := range names {
		if !expected[name] {
			return "an extension migration ledger is absent from the composed test manifest; run the integration lane from this worktree", nil
		}
	}
	return "", nil
}
