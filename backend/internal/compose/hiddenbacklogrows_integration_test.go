// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The hidden-backlog rows through the PRODUCTION binding.
//
// The module's own tests prove the difference against real rows, and the
// service's unit tests prove the projection over a scripted seam. Neither
// proves the wiring between them: a compose adapter that never reached the
// store, or reached it and dropped the rows, would leave both green. That gap
// is what this closes — newAttentionService is what the route itself serves.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
)

// A rule nothing hides answers an empty list rather than failing, and it
// travels the whole way: handler vocabulary in, module difference out.
func TestTheHiddenRowsReadReachesTheStoreThroughTheSeam(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now()
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })

	got, err := svc.HiddenBacklogRows(e.Admin(), "set_aside")
	if err != nil {
		t.Fatalf("reading the rows behind set_aside: %v", err)
	}

	if string(got.Rule) != "set_aside" {
		t.Errorf("echoed rule %q, want the one asked for", got.Rule)
	}
	if got.Rows == nil {
		t.Error("answered a null row list where an empty one is the fact")
	}
	if got.AsOf.IsZero() {
		t.Error("answered no instant; the figure and its rows must name the same read")
	}
}

// Every rule the counts carry is reachable through the same wiring. A rule that
// reached the store and was refused there would be a vocabulary the two halves
// disagree about — which is exactly what the shared rule words exist to stop.
func TestEveryCountedRuleIsReadableThroughTheSeam(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now()
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })

	for _, rule := range []string{"set_aside", "not_sales", "past_horizon", "unlinked", "colleagues"} {
		if _, err := svc.HiddenBacklogRows(e.Admin(), rule); err != nil {
			t.Errorf("%s: %v", rule, err)
		}
	}
}

// A rule the guardrail does not measure is refused rather than answered empty.
// "Nothing is behind that rule" and "there is no such rule" are different
// answers, and the first given for the second lets a typo read as a clean queue.
func TestAnUnknownRuleIsRefusedThroughTheSeam(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now()
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })

	if _, err := svc.HiddenBacklogRows(e.Admin(), "no_such_rule"); err == nil {
		t.Fatal("an unknown rule was answered rather than refused")
	}
}

// The tier gate holds at the seam too, not only over a scripted lane: a reader
// who sees only their own work is told they may not ask, rather than handed an
// empty list they would read as a clear queue.
func TestAnOwnScopedReaderIsRefusedTheRowsThroughTheSeam(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now()
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })

	ctx := context.Context(e.As(e.Rep1, nil, integration.RepPerms))

	if _, err := svc.HiddenBacklogRows(ctx, "set_aside"); err == nil {
		t.Fatal("an own-scoped reader was given a lead's reading")
	}
}
