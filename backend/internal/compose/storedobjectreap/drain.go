// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storedobjectreap

import (
	"context"
	"time"
)

// Batch reaps one read of the ledger. It answers how many keys it read and how
// many it settled. A key is settled when its bytes are deleted, or when its
// intent is retired because a row names it.
type Batch func(ctx context.Context) (listed, settled int, err error)

// Drain takes batches until one comes back shorter than size, settles no key,
// or ends at or after stop. A batch that settles nothing holds only keys the
// store refused to delete. Reading it again would delete nothing more.
func Drain(ctx context.Context, batch Batch, size int, stop time.Time, now func() time.Time) error {
	for {
		listed, settled, err := batch(ctx)
		if err != nil || listed < size || settled == 0 || !now().Before(stop) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
