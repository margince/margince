// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// `worker replay-erasures` re-erases the contacts a restore brought back.
//
// A subcommand rather than an endpoint or a job. An operator runs it from a
// shell right after a restore, when the api may not be serving yet. A queued
// job would read a queue that is itself restored state.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// runReplayErasures builds the eraser the api erases with, from the same
// environment. The replay then reaches attachment bytes, sealed link material
// and provider originals, as an Art. 17 erasure does.
//
// The vault is resolved here and not by the dispatcher, so the other database
// subcommands run without one, as they always have.
func runReplayErasures(
	ctx context.Context, pool *pgxpool.Pool, resolveVault func() (keyvault.Vault, error),
	settings *config.Config, args []string, stdout io.Writer,
) error {
	fs := flag.NewFlagSet("replay-erasures", flag.ContinueOnError)
	fs.SetOutput(stdout)
	emptyJournalOK := fs.Bool("empty-journal-ok", false,
		"run even when the erasure journal holds no entry; only for an installation that has never erased anyone")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("replay-erasures: unexpected argument %q; usage: worker replay-erasures [--empty-journal-ok]", fs.Arg(0))
	}
	blob, _, err := blobstore.FromConfig(ctx, settings)
	if err != nil {
		return fmt.Errorf("replay-erasures: blobstore: %w", err)
	}
	vault, err := resolveVault()
	if err != nil {
		return err
	}
	replay, err := compose.ReplayErasureSuppressions(ctx, pool, blob, vault, *emptyJournalOK)
	if errors.Is(err, compose.ErrReplayNeedsObjectStore) || errors.Is(err, compose.ErrReplayJournalEmpty) {
		return err
	}
	// The report prints even when a contact was refused: the erasures before it
	// are committed, and the operator needs to know how many there were.
	writeErasureReplayReport(stdout, replay)
	return err
}

// writeErasureReplayReport says what the replay took and, as plainly, what it
// could not cover.
func writeErasureReplayReport(stdout io.Writer, replay compose.ErasureReplay) {
	if replay.JournalEmpty {
		_, _ = fmt.Fprintln(stdout, "The erasure journal holds no entry; the replay read only the restored database's own list.")
	} else {
		newest := replay.Journal.NewestErasure.Format(time.RFC3339)
		_, _ = fmt.Fprintf(stdout, "The erasure journal knows erasures up to %s, in %d entries.\n", newest, replay.Journal.Entries)
		_, _ = fmt.Fprintf(stdout, "Anything erased after %s is NOT covered. "+
			"Erase those contacts again by hand, from your record of the erasure requests.\n", newest)
	}
	_, _ = fmt.Fprintf(stdout, "Re-erased %d contact(s) that the suppression list or its journal names.\n", replay.Taken)
}
