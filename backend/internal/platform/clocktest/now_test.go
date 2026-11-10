// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package clocktest

import (
	"testing"
	"time"
)

func TestNowCarriesNoMoreThanTheMicrosecondsPostgresStores(t *testing.T) {
	for range 100 {
		now := Now(t)
		if !now.Equal(now.Truncate(time.Microsecond)) {
			t.Fatalf("Now() = %v carries sub-microsecond digits that a stored copy would lose", now)
		}
	}
}

// A fixture N days back is then N times 24 hours back, as the database counts
// it, in whatever zone the process runs.
func TestNowIsInUTC(t *testing.T) {
	if got := Now(t).Location(); got != time.UTC {
		t.Fatalf("Now() is in %v, want UTC", got)
	}
}
