// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// contactEvent builds the envelope the consumer reads.
func contactEvent(eventType string, id ids.UUID) events.Envelope {
	return events.Envelope{
		Type:   eventType,
		Entity: events.EntityRef{Type: "contact", ID: id},
	}
}

// The consumer must ignore what it cannot act on WITHOUT touching the pool.
// The nil pool is the assertion: anything that reached the database here would
// panic, so a pass proves the refusal happened at the envelope.
func TestContactAutoEnrichIgnoresWhatItCannotActOn(t *testing.T) {
	g := &ContactAutoEnrich{}
	ctx := context.Background()

	for name, env := range map[string]events.Envelope{
		"an archived contact needs no match — the match requires a live row": contactEvent("contact.archived", ids.NewV7()),
		"an unrelated entity type":                                       {Type: "contact.created", Entity: events.EntityRef{Type: "company", ID: ids.NewV7()}},
		"an event about no entity at all":                                contactEvent("contact.created", ids.Nil),
		"a verb outside the set that can make a contact newly matchable": contactEvent("contact.disqualified", ids.NewV7()),
	} {
		if err := g.HandleEvent(ctx, env); err != nil {
			t.Errorf("%s: HandleEvent returned %v, want a silent skip", name, err)
		}
	}
}

// The fill is not a human's edit and must never be recorded as one: the rows
// it writes carry this principal into captured_by, and a human id there would
// make an automatic fill indistinguishable from something a rep typed.
func TestContactAutoEnrichWritesAsASystemPrincipal(t *testing.T) {
	g := &ContactAutoEnrich{}
	ws := ids.NewV7()
	env := events.Envelope{
		Type:   "contact.created",
		Entity: events.EntityRef{Type: "contact", ID: ids.NewV7()},
	}

	ctx := g.systemContext(context.Background(), env, ws)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the pass writes with no principal at all")
	}
	if actor.Type != principal.PrincipalSystem {
		t.Errorf("actor type = %q, want a system principal", actor.Type)
	}
	if got, ok := principal.WorkspaceID(ctx); !ok || got != ws {
		t.Error("the workspace did not bind, so every write would land outside RLS's reach")
	}
}
