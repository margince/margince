// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"slices"
	"testing"
)

func TestStopLocksMeetEveryPlainAndHashedReaderMatch(t *testing.T) {
	addresses := []string{" Buyer@BÜCHER.example ", "buyer@xn--bcher-kva.example"}
	lowered, hashes := StopAddressArgs(addresses...)
	if len(lowered) == 0 || len(hashes) == 0 {
		t.Fatal("the reader lost its plain or hashed address matches")
	}
	keys := StopAddressLockKeys(addresses, nil)
	if len(keys) == 0 || !slices.IsSorted(keys) {
		t.Fatalf("reader keys are empty or unordered: %v", keys)
	}
	for _, address := range lowered {
		if !overlappingStopKeys(keys, StopAddressLockKeys([]string{address}, nil)) {
			t.Fatalf("plain reader match %q has no shared lock", address)
		}
	}
	for _, hash := range hashes {
		if !overlappingStopKeys(keys, StopAddressLockKeys(nil, []string{hash})) {
			t.Fatal("a hash-only stop has no lock shared with the reader")
		}
	}
	if overlappingStopKeys(keys, StopAddressLockKeys([]string{"elsewhere@another.example"}, nil)) {
		t.Fatal("unrelated recipients share a stop lock")
	}
	repeated := StopAddressLockKeys(append(addresses, addresses...), append(hashes, hashes...))
	if !slices.Equal(keys, repeated) {
		t.Fatal("repeated reader matches change the ordered lock set")
	}
}

func overlappingStopKeys(left, right []string) bool {
	for _, key := range left {
		if slices.Contains(right, key) {
			return true
		}
	}
	return false
}
