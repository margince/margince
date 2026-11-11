// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"errors"
	"io"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: migrate <up|down|reset-password|setup-token|recreate-db|drop-db|db-exists|workspace-exists|drill-start|drill-finish|data-tasks|extension-manifest> --dsn <dsn> [--steps n] [--email <address>] [--name <db>] [--template <db>] [--statement-timeout <duration>] [--by <name>] [--restored-to <time>] [--drill <id>] [--outcome passed|failed] [--note <text>]")
	}
	direction := args[0]
	if direction == "extension-manifest" {
		return writeExtensionManifest(args[1:], stdout)
	}
	if direction == "data-tasks" {
		return runDataTasks(ctx, args[1:], stdout, stderr)
	}

	return runMigration(ctx, args, stdout, stderr)
}
