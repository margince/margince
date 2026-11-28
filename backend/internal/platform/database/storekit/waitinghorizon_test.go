// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The reader compares the stored instant with a bound the driver cuts to the
// microsecond. A stored 999 nanoseconds more would round up past that bound.
func TestAWaitingHorizonIsStoredAtTheMicrosecondTheDatabaseHolds(t *testing.T) {
	tx := &fakeTx{}
	measured := time.Date(2026, 5, 4, 9, 30, 0, 123456999, time.UTC)
	if err := StoreWaitingHorizonTx(context.Background(), tx, 21, measured); err != nil {
		t.Fatal(err)
	}
	stored, encoded := tx.execArgs[1].(string)
	if !encoded || !strings.Contains(stored, `"measured_at":"2026-05-04T09:30:00.123456Z"`) {
		t.Fatalf("the horizon was stored as %v, want the instant cut to the microsecond", tx.execArgs[1])
	}
}

// The shared table has no CHECK on the days, so the writer is what refuses them.
func TestAWaitingHorizonUnderOneDayIsNotStored(t *testing.T) {
	for _, days := range []int{0, -3} {
		// No transaction is passed: the refusal must come before any write.
		if err := StoreWaitingHorizonTx(context.Background(), nil, days, time.Unix(0, 0)); err == nil {
			t.Errorf("a horizon of %d days was accepted, want it refused", days)
		}
	}
}
