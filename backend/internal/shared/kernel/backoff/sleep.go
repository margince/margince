// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backoff

import (
	"context"
	"time"
)

// Sleep waits d, or gives up when the caller does: a wait nobody is still
// owed returns the context's error instead of holding the goroutine.
func Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
