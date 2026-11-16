// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package snapshotreads

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnIDSetIsOrderAndDuplicateFree(t *testing.T) {
	a, b := ids.NewV7(), ids.NewV7()
	given := []ids.UUID{b, a, b}
	before := slices.Clone(given)
	if IDSet(given) != IDSet([]ids.UUID{a, b}) {
		t.Fatalf("the same set in another order or with a repeat is another key: %q, %q", IDSet(given), IDSet([]ids.UUID{a, b}))
	}
	if IDSet([]ids.UUID{a}) == IDSet([]ids.UUID{a, b}) {
		t.Fatal("a smaller set shares the key of a larger one")
	}
	if !slices.Equal(given, before) {
		t.Fatal("building the key reordered the caller's ids")
	}
}

func TestAReadWithoutAnOpenMemoAlwaysReads(t *testing.T) {
	reads := 0
	for range 2 {
		if _, err := Read(context.Background(), nil, "k", func() (int, error) { reads++; return reads, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if reads != 2 {
		t.Fatalf("read %d times without a memo; want 2", reads)
	}
}
