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

// The firing records its owner as the human it acts for.
//
// This is what reaches approval.on_behalf_of when the firing stages a held
// draft, and it is what the decision-authority predicate narrows that draft by:
// releasing one sends it from the approver's own mailbox, so only the person it
// goes out as may release it. A staging with nobody recorded is decidable by
// nobody, so an unstamped firing does not merely lose a nicety — it strands the
// card.
//
// OnBehalfOf rather than UserID, deliberately. UserID is the row-scope key and
// setting it would widen what the firing may read; OnBehalfOf is read by the
// audit trail and the staging layer, and auth.AuthzRule short-circuits on the
// system principal before ever reaching it.
func TestAFiringRecordsItsOwnerAsTheHumanItActsFor(t *testing.T) {
	owner := ids.NewV7()
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: systemActor})

	actor, ok := principal.Actor(withSendingOwner(ctx, workflow.Event{OwnerID: owner}))
	if !ok {
		t.Fatal("the firing carries no acting principal at all")
	}
	if actor.OnBehalfOf != owner {
		t.Errorf("the firing acts on behalf of %s, want its owner %s. Anything it stages records that "+
			"value, and a held draft recording nobody can be released by nobody", actor.OnBehalfOf, owner)
	}
}

// A system-seeded firing records nobody, and its staging stays decidable on
// grants alone.
//
// The pair to the test above. Inventing an owner here would hand one arbitrary
// human the sole right to release what a firing nobody authored composed — and
// would do it by guessing, which is the one move a message to a customer cannot
// afford.
func TestASystemSeededFiringRecordsNoHuman(t *testing.T) {
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: systemActor})

	actor, ok := principal.Actor(withSendingOwner(ctx, workflow.Event{}))
	if !ok {
		t.Fatal("the firing carries no acting principal at all")
	}
	if actor.OnBehalfOf != ids.Nil {
		t.Errorf("a firing no human authored acts on behalf of %s; there is nobody it could honestly "+
			"name, and naming one decides whose message it is by guessing", actor.OnBehalfOf)
	}
}
