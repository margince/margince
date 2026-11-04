// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package auth

import (
	"context"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordChecksForTest returns ctx carrying a recorder for its principal, and a
// func listing every check a gate has asked of that principal since. It lives
// behind the integration tag, so no shipped binary links it.
func RecordChecksForTest(ctx context.Context) (context.Context, func() []string) {
	rec := &checkRecorder{checks: map[string]bool{}}
	if p, ok := principal.Actor(ctx); ok {
		rec.actor = p.ID
	}
	return context.WithValue(ctx, checkRecorderKey{}, rec), func() []string {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return slices.Sorted(maps.Keys(rec.checks))
	}
}
