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

// RecordedCall is what the gates asked of one caller during one call, and
// which of them refused it without a 403.
type RecordedCall struct {
	Checks  []string
	Refused []string
}

// RecordChecksForTest returns ctx carrying a recorder for its principal, and a
// func reading what the gates have noted of that principal since. It lives
// behind the integration tag, so no shipped binary links it.
func RecordChecksForTest(ctx context.Context) (context.Context, func() RecordedCall) {
	rec := &checkRecorder{checks: map[string]bool{}, refused: map[string]bool{}}
	if p, ok := principal.Actor(ctx); ok {
		rec.actor = p.ID
	}
	return context.WithValue(ctx, checkRecorderKey{}, rec), func() RecordedCall {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return RecordedCall{Checks: slices.Sorted(maps.Keys(rec.checks)), Refused: slices.Sorted(maps.Keys(rec.refused))}
	}
}
