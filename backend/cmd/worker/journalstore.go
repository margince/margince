// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
)

// bindStores sets the lanes' attachment store, and the store the hourly export
// writes the erasure journal to, through the resolver the replay also uses.
func (l *workerLanes) bindStores(
	ctx context.Context, settings *config.Config, blob blobstore.Store, stdout io.Writer,
) error {
	l.blob = blob
	journal, separate, err := compose.ResolveJournalStore(ctx, settings, blob)
	if err != nil {
		return fmt.Errorf("worker: %w", err)
	}
	l.journal = journal
	if separate {
		_, _ = fmt.Fprintln(stdout, "worker exporting the erasure journal to its own object store")
	}
	return nil
}
