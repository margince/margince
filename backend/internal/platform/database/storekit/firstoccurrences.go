// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// FirstOccurrences returns ids in order with each repeat dropped, keeping its
// first place. A list kept as one row per id stores this list. Its writer
// answers with it too, so the answer matches what a later read returns.
func FirstOccurrences[T comparable](ids []T) []T {
	seen := make(map[T]struct{}, len(ids))
	kept := make([]T, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		kept = append(kept, id)
	}
	return kept
}
