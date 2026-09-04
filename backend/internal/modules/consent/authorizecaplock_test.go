// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The order the cap takes its address locks in, which is what stops two
// messages from deadlocking each other.

import (
	"slices"
	"strings"
	"testing"
)

// Two messages naming the same addresses in opposite orders must take the same
// locks in the SAME order, or Postgres resolves the crossing by killing one
// transaction and a legitimate send burns its retry ladder — a deadlock a
// caller can provoke just by choosing a To order.
//
// Asserted on the keys rather than against a live database because the property
// is the ordering itself: sorting the keys is what makes the crossing
// impossible to express, and a test that only ran two transactions would pass
// whenever the timing happened not to cross.
func TestTwoMessagesTakeTheSameAddressLocksInTheSameOrder(t *testing.T) {
	forward := capLockKeys([]string{"a@x.test", "b@x.test", "c@x.test"})
	reversed := capLockKeys([]string{"c@x.test", "b@x.test", "a@x.test"})
	if !slices.Equal(forward, reversed) {
		t.Fatalf("the same addresses lock in different orders: %v vs %v", forward, reversed)
	}
	if !slices.IsSorted(forward) {
		t.Errorf("lock keys are not sorted: %v", forward)
	}
}

// One mailbox is one lock however it is spelled, or a message naming both
// spellings would take two locks for one address and a concurrent message
// naming one spelling could slip between them.
func TestOneMailboxIsOneLock(t *testing.T) {
	keys := capLockKeys([]string{" Buyer@X.test ", "buyer@x.test"})
	if len(keys) != 1 {
		t.Fatalf("two spellings of one mailbox produced %d locks, want 1", len(keys))
	}
}

// The namespace occupies the high half and the hash the low half, so a cap lock
// can never collide with another subsystem's advisory lock on a small integer,
// and the key is always positive — a negative key is legal in Postgres but
// makes a lock impossible to recognise in pg_locks.
func TestTheLockKeyIsNamespacedAndPositive(t *testing.T) {
	for _, address := range []string{"a@x.test", "zzzzzzzz@example.test", ""} {
		key := capLockKey(address)
		if key <= 0 {
			t.Errorf("capLockKey(%q) = %d, want a positive key", address, key)
		}
		if key&^0xFFFFFFFF != capLockNamespace {
			t.Errorf("capLockKey(%q) high half = %#x, want the namespace %#x",
				address, key&^0xFFFFFFFF, capLockNamespace)
		}
	}
}

// The lock key and the count must agree on what a mailbox is, or an address
// could be locked under one spelling and counted under another — and the
// ceiling would bind to neither.
func TestTheLockAndTheCountNormalizeAlike(t *testing.T) {
	const address = "  Buyer@X.Test  "
	if got := normalizeCapAddress(address); got != strings.ToLower(strings.TrimSpace(address)) {
		t.Fatalf("normalizeCapAddress(%q) = %q", address, got)
	}
	if capLockKey(address) != capLockKey("buyer@x.test") {
		t.Error("the lock key differs between two spellings of one mailbox")
	}
}
