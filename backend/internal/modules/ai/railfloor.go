// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "context"

type floorKey struct{}

// WithFloor declares that the site making the calls under ctx answers with its
// own deterministic result when a call fails: a plain draft, a copied offer, a
// brief built from the record. The reader still gets what they asked for, so
// the rail settles such a failure as degraded rather than failed.
//
// Declared where the floor is served, and only there: a site that returns the
// error to its caller has nothing to show, and its failure stays a failure.
func WithFloor(ctx context.Context) context.Context {
	return context.WithValue(ctx, floorKey{}, true)
}

// ServesFloor reports whether a site under ctx declared a floor.
func ServesFloor(ctx context.Context) bool {
	declared, ok := ctx.Value(floorKey{}).(bool)
	return ok && declared
}
