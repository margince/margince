// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// An automation's draft is written in its owner's voice.
//
// The firing runs under the system actor so its writes stay attributed to the
// system, and that is also why the drafter could not tell whose voice to write
// in: a system principal names no person. The owner is bound alongside it, for
// that one question and nothing else.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// A human-authored firing names its owner as the sender.
func TestAFiringNamesItsOwnerAsTheSendingHuman(t *testing.T) {
	owner := ids.NewV7()
	// The system actor the engine really binds, so the test measures the
	// context production assembles rather than an empty one.
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: systemActor})

	got, ok := principal.SendingHuman(withSendingOwner(ctx, workflow.Event{OwnerID: owner}))
	if !ok {
		t.Fatal("a firing owned by a human binds no sending human, so anything it drafts is written " +
			"in nobody's voice")
	}
	if got != owner {
		t.Errorf("the draft would be written in %s's voice, want the automation's owner %s", got, owner)
	}
}

// A system-seeded firing names nobody.
//
// The other half, and the one that keeps the binding honest: an automation no
// human authored has no voice to borrow, and inventing one would sign a message
// in a person who never asked for it.
func TestASystemSeededFiringNamesNoSendingHuman(t *testing.T) {
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: systemActor})

	if _, ok := principal.SendingHuman(withSendingOwner(ctx, workflow.Event{})); ok {
		t.Error("a firing with no owner names a sending human anyway, so a message no human authored " +
			"would go out written in somebody's voice")
	}
}

// Binding the sender moves nothing else.
//
// The acting principal is what audit, row scope and every permission check read.
// A fix that bound the owner as the ACTOR would have made an automation's writes
// look like the owner's own and widened what it may touch to whatever they may
// touch — a far larger claim than "sign this in their voice".
func TestBindingTheSenderLeavesTheActingPrincipalAlone(t *testing.T) {
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: systemActor})

	actor, ok := principal.Actor(withSendingOwner(ctx, workflow.Event{OwnerID: ids.NewV7()}))
	if !ok {
		t.Fatal("binding the sender dropped the acting principal")
	}
	if actor.Type != principal.PrincipalSystem {
		t.Errorf("the firing now acts as %q; an automation's writes must stay attributed to the system "+
			"actor, not to its owner", actor.Type)
	}
	if !actor.UserID.IsZero() {
		t.Error("the acting principal gained a UserID, which is a row-scope key — binding the sender " +
			"must not widen what the firing may read or write")
	}
}
