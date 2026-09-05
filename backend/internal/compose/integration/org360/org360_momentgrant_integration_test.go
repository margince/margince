// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package org360

// The account card's verb, against the grant its form actually needs.
//
// The card gates on activity.READ — a reader who may see what is owed gets
// told what is owed. The verb under it posts to /activities, which needs
// activity.CREATE. The two grants are separate in the seeded matrix, so the
// gap between them is a real seat and not a hypothetical one.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// org360LogReaderPerms is the full account rep with the one grant the card's
// verb needs taken away, and nothing else changed: whatever the assertions
// below see is the activity WRITE grant answering, not a narrower page.
var org360LogReaderPerms = withActivityReadOnly(integration.AccountRepPerms)

func withActivityReadOnly(base principal.Permissions) principal.Permissions {
	narrowed := base
	narrowed.Objects = make(map[string]principal.ObjectGrant, len(base.Objects))
	for object, grant := range base.Objects {
		narrowed.Objects[object] = grant
	}
	narrowed.Objects["activity"] = principal.ObjectGrant{Read: true}
	return narrowed
}

func TestTheAccountCardWithholdsItsLogVerbFromAReaderWhoCannotLog(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)
	orgID := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Acme", &e.Rep1))

	// A fresh account owes nothing, which is the rung that recommends logging
	// something — the card's only verb that writes.
	page, err := svc.Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, org360LogReaderPerms), orgID)
	if err != nil {
		t.Fatalf("assemble for a reader holding activity.read alone: %v", err)
	}
	if page.Moment == nil {
		t.Fatal("no moment card for a reader who holds activity.read — the card gates on READ, " +
			"and without it this test proves nothing about the verb under it")
	}
	action := page.Moment.RecommendedAction
	if action.Kind != crmcontracts.PersonMomentActionKindLogActivity {
		t.Fatalf("recommended action kind = %q, want log_activity — a fresh account owes nothing, "+
			"so the quiet rung should have fired", action.Kind)
	}
	if action.State != crmcontracts.PersonMomentActionStateBlocked {
		t.Errorf("recommended action state = %q, want blocked: this reader's POST /activities is refused, "+
			"so an actionable verb is a button that silently does nothing", action.State)
	}
	if action.BlockedReason == nil || *action.BlockedReason == "" {
		t.Error("the withheld verb carries no reason; blocked without a sentence is the same dead " +
			"button wearing a different state")
	}
	if action.Destination != nil {
		t.Errorf("the withheld verb kept its destination %+v — the client routes on that field", action.Destination)
	}

	// The control: the same account and the same rung for a rep who may log.
	// Without it, a card that stopped firing at all would pass everything above.
	granted, err := svc.Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms), orgID)
	if err != nil {
		t.Fatalf("assemble for a rep who may log: %v", err)
	}
	if granted.Moment == nil {
		t.Fatal("no moment card for a fully granted rep")
	}
	if got := granted.Moment.RecommendedAction.State; got != crmcontracts.PersonMomentActionStateWillConfirm {
		t.Errorf("recommended action state = %q for a rep who may log, want the will_confirm the "+
			"card is minted with", got)
	}
}
