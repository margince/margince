// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package principal

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAgentRunIDRoundTrips(t *testing.T) {
	id := ids.NewV7()
	ctx := WithAgentRunID(context.Background(), id)
	got, ok := AgentRunID(ctx)
	if !ok || got != id {
		t.Fatalf("AgentRunID = %v, %v; want %v, true", got, ok, id)
	}
	if _, ok := AgentRunID(context.Background()); ok {
		t.Fatal("AgentRunID on a bare context should report ok=false")
	}
}

// The subject label is what lets the rail name the record the AI is working
// on, and the two things worth pinning are the round trip and that an empty
// name binds NOTHING: a caller with no name to give must leave the generic
// sentence standing, not hand the rail an empty pair of quotes.
func TestWorkSubjectRoundTripsAndIgnoresAnEmptyName(t *testing.T) {
	ctx := WithWorkSubject(context.Background(), "Anna Berg")
	got, ok := WorkSubject(ctx)
	if !ok || got != "Anna Berg" {
		t.Fatalf("WorkSubject = %q, %v; want %q, true", got, ok, "Anna Berg")
	}
	if _, ok := WorkSubject(context.Background()); ok {
		t.Fatal("WorkSubject on a bare context should report ok=false")
	}
	if _, ok := WorkSubject(WithWorkSubject(context.Background(), "  ")); ok {
		t.Fatal("a blank label must bind nothing — the rail would render it as an empty name")
	}
}
