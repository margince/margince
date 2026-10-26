// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

import (
	"fmt"
	"os"
)

// syncDir makes a rename durable.
//
// Syncing the file only makes its BYTES durable: the rename is a change to the
// directory entry, and a crash can lose it while the staging file's contents
// survive. The object then does not exist under the key a row already points at
// — which is the exact failure the fsync in fillStaging was there to prevent, so
// stopping at the file would have been half a promise.
func syncDir(dir string) error {
	handle, err := os.Open(dir) //nolint:gosec // a directory this package created under its own root
	if err != nil {
		return fmt.Errorf("blobstore: open %s to sync: %w", dir, err)
	}
	// Close runs whether or not the sync failed, and either failure is reported.
	if err := errorsJoin(handle.Sync(), handle.Close()); err != nil {
		return fmt.Errorf("blobstore: sync %s: %w", dir, err)
	}
	return nil
}
