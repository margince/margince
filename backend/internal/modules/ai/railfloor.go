// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "context"

type floorKey struct{}

// WithFloor declares that the site answers with its own result when a call
// under ctx fails (a plain draft, a copied offer). The rail then settles the
// failure as degraded: the reader still got what they asked for.
func WithFloor(ctx context.Context) context.Context {
	return context.WithValue(ctx, floorKey{}, true)
}

// ServesFloor reports whether a site under ctx declared a floor.
func ServesFloor(ctx context.Context) bool {
	declared, ok := ctx.Value(floorKey{}).(bool)
	return ok && declared
}
