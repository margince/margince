// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// `worker replay-erasures` re-erases the contacts a restore brought back.
//
// A subcommand rather than an endpoint or a job: an operator runs it from a
// shell right after a restore, when the api may not be serving yet, and a
// restored queue is itself restored state.

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// runReplayErasures builds the same eraser the api erases with, from the same
// environment, so the replay reaches attachment bytes, sealed link material
// and provider originals as an Art. 17 erasure does.
func runReplayErasures(ctx context.Context, pool *pgxpool.Pool, env config.Lookup, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("replay-erasures", flag.ContinueOnError)
	fs.SetOutput(stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}
	blob, _, err := blobstore.FromEnv(ctx, env)
	if err != nil {
		return fmt.Errorf("replay-erasures: blobstore: %w", err)
	}
	vault, err := keyvault.ForRole(ctx, "replay-erasures", pool, env)
	if err != nil {
		return err
	}
	taken, err := compose.ReplayErasureSuppressions(ctx, pool, blob, vault)
	// The count prints even when a contact was refused: the erasures before it
	// are committed, and the operator needs to know how many there were.
	_, _ = fmt.Fprintf(stdout, "Re-erased %d contact(s) that the suppression list or its journal names.\n", taken)
	return err
}
