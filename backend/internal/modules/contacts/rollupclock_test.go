// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// One clock decides which FX rate an account's pipeline converts at. It hands
// over an instant, and SQL cuts the installation's day from it.

import (
	"context"
	"testing"
	"time"
)

func TestTheAsOfInstantComesFromTheInjectedClockUnchanged(t *testing.T) {
	t.Parallel()
	// 23:30 in Los Angeles on the 2nd is already the 3rd in UTC and Auckland.
	// Handing over the instant keeps every one of those days derivable.
	frozen := time.Date(2026, 6, 3, 6, 30, 0, 0, time.UTC)
	store := (&Store{}).SetClock(func() time.Time { return frozen })

	if got := rollupAsOf(store.withRollupAsOf(context.Background())); !got.Equal(frozen) {
		t.Errorf("as-of = %s, want %s: an instant cut to a UTC day here would name the wrong "+
			"day for an installation west or east of UTC", got, frozen)
	}
}
