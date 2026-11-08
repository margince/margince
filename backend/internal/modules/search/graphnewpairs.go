// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// A missing pair can lose its first insert to a newer fold while its statement waits.
// Read that bounded subset again after the upsert owns its rows.
func recomputeContactPairs(ctx context.Context, tx pgx.Tx, pairs, missing []contactPair) error {
	written, err := foldContactPairs(ctx, tx, pairs)
	if err != nil {
		return err
	}
	_, err = foldContactPairs(ctx, tx, intersectGraphPairs(written, missing))
	return err
}

func recomputePairs(ctx context.Context, tx pgx.Tx, pairs, missing []pair) error {
	written, err := foldPairs(ctx, tx, pairs)
	if err != nil {
		return err
	}
	_, err = foldPairs(ctx, tx, intersectGraphPairs(written, missing))
	return err
}

func intersectGraphPairs[T comparable](pairs, missing []T) []T {
	if len(missing) == 0 || len(pairs) == 0 {
		return nil
	}
	selected := make(map[T]struct{}, len(pairs))
	for _, p := range pairs {
		selected[p] = struct{}{}
	}
	var newlyLocked []T
	for _, p := range missing {
		if _, ok := selected[p]; ok {
			newlyLocked = append(newlyLocked, p)
		}
	}
	return newlyLocked
}
