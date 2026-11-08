// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package auth

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordedCall is what the gates asked of one caller during one call, and
// which of them refused it. Human is the user the caller is or acts for.
type RecordedCall struct {
	Checks  []string
	Refused []string
	Human   string
}

// RecordChecksForTest returns ctx carrying a recorder for its principal, and a
// func reading what the gates have noted of that principal since. It lives
// behind the integration tag, so no shipped binary links it.
func RecordChecksForTest(ctx context.Context) (context.Context, func() RecordedCall) {
	rec := &checkRecorder{checks: map[string]bool{}, refused: map[string]bool{}}
	var human string
	if p, ok := principal.Actor(ctx); ok {
		rec.actor = p.ID
		human = p.UserID.String()
		if p.Type == principal.PrincipalAgent {
			human = p.OnBehalfOf.String()
		}
	}
	return context.WithValue(ctx, checkRecorderKey{}, rec), func() RecordedCall {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return RecordedCall{Checks: slices.Sorted(maps.Keys(rec.checks)), Refused: slices.Sorted(maps.Keys(rec.refused)), Human: human}
	}
}

// RefusedInItsOwnWords reports whether a gate answers a recorded refusal with
// its contract's status, not a 403. An all-scope seat asking for a record it
// may change is a 409; a receiver outside the caller's scope is a 422.
func RefusedInItsOwnWords(refusal string) bool {
	return strings.HasPrefix(refusal, narrowerPrefix) || refusal == assigneeRefusal
}
