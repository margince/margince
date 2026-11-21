// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"testing"
	"time"
)

// The shared table has no CHECK on the days, so the writer is what refuses them.
func TestAWaitingHorizonUnderOneDayIsNotStored(t *testing.T) {
	for _, days := range []int{0, -3} {
		// No transaction is passed: the refusal must come before any write.
		if err := StoreWaitingHorizonTx(context.Background(), nil, days, time.Unix(0, 0)); err == nil {
			t.Errorf("a horizon of %d days was accepted, want it refused", days)
		}
	}
}
