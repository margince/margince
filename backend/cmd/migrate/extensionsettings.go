// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/pkg/extension"
)

// seedExtensionSettings applies the composed units' declared settings and
// prints what each unit stored and kept. It is a verb of this process, not a
// route. An operator or a deploy script runs it with the owner's DSN and no
// signed-in member.
func seedExtensionSettings(ctx context.Context, conn *pgx.Conn, exts []extension.Extension, stdout io.Writer) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate extension-settings: %w", err)
	}
	//craft:ignore swallowed-errors rollback after a commit is a no-op, and after a failure the original error is the one to report
	defer func() { _ = tx.Rollback(ctx) }()

	reports, err := settings.SeedExtensions(ctx, tx, exts)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate extension-settings: %w", err)
	}
	for _, r := range reports {
		if _, err := fmt.Fprintf(stdout, "%s: stored %d, kept %d\n", r.Unit, len(r.Stored), len(r.Kept)); err != nil {
			return fmt.Errorf("migrate extension-settings: writing the report: %w", err)
		}
	}
	return nil
}
