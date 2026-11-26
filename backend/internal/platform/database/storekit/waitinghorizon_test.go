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
	tx := &stateTx{rows: map[string][]byte{}}
	measured := time.Date(2026, 5, 4, 9, 30, 0, 123456999, time.UTC)
	if err := StoreWaitingHorizonTx(context.Background(), tx, 21, measured); err != nil {
		t.Fatal(err)
	}
	stored := string(tx.rows[string(StateActivityWaitingHorizon)])
	if !strings.Contains(stored, `"measured_at":"2026-05-04T09:30:00.123456Z"`) {
		t.Fatalf("the horizon was stored as %s, want the instant cut to the microsecond", stored)
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
