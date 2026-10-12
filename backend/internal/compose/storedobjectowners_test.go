// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/storedobjects"
)

// Every kind a writer may declare has an owner that can say whether its keys are
// still referenced.
//
// The two sides fail silently apart in the direction that costs most: a writer
// records intents under a kind the sweep cannot adjudicate, the sweep reports
// nothing, and finding no orphans is exactly what a healthy tree looks like. The
// bytes then sit where no erasure reaches them, which is the whole defect.
func TestEveryStoredObjectKindHasAnOwner(t *testing.T) {
	t.Parallel()
	owners := unreferencedKeysBy(nil)
	for _, kind := range storedobjects.AllKinds() {
		if _, ok := owners[kind]; !ok {
			t.Errorf("kind %q is declarable but no owner answers whether its keys are referenced, "+
				"so the sweep can never adjudicate one", kind)
		}
	}
	// And nothing beyond the vocabulary: an owner for a kind no writer declares is a
	// table being asked about keys that never arrive.
	declared := map[storedobjects.Kind]bool{}
	for _, kind := range storedobjects.AllKinds() {
		declared[kind] = true
	}
	for kind := range owners {
		if !declared[kind] {
			t.Errorf("an owner is registered for kind %q, which no writer may declare", kind)
		}
	}
}
