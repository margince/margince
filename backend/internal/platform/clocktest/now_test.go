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
