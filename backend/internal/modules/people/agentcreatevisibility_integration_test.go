// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Who can see a contact an AGENT typed in.
//
// A create through the tool surface reaches the same store method the UI and
// the REST API reach, so the only thing that ever distinguished them was the
// principal behind the call. It used to: an agent-created contact was born
// owner-scoped and no colleague could see it, which reads to the person who
// asked for it as the contact simply not being there.
//
// The capture paths are the ones that still mint owner-scoped contacts, and
// they say so on the spec rather than being inferred from who is acting.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAContactAnAgentCreatesBelongsToTheWorkspace(t *testing.T) {
	e := setupCapturePrivacy(t)

	created, err := e.store.CreatePerson(e.asAgent(e.owner), CreatePersonInput{
		FullName: "Lucy Vo",
		Emails:   []PersonEmailInput{{Email: "lucy.vo@kunde.example", EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("creating a person as an agent: %v", err)
	}

	if got := e.visibilityOf(t, ids.From[ids.PersonKind](ids.UUID(created.Id))); got != visibilityWorkspace {
		t.Errorf("a contact an agent created is %q, want workspace: the rep was told it exists "+
			"and no colleague can see it, which is indistinguishable from it never being created",
			got)
	}
}

// The control beside it. A human create was always `workspace`, and the point
// of the change is that the two doors now agree — so a test that only checked
// the agent could pass against a store that published nothing at all.
func TestAContactAHumanCreatesBelongsToTheWorkspace(t *testing.T) {
	e := setupCapturePrivacy(t)

	created, err := e.store.CreatePerson(e.as(e.owner, principal.RowScopeAll), CreatePersonInput{
		FullName: "Marta Kern",
		Emails:   []PersonEmailInput{{Email: "marta.kern@kunde.example", EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("creating a person as a human: %v", err)
	}

	if got := e.visibilityOf(t, ids.From[ids.PersonKind](ids.UUID(created.Id))); got != visibilityWorkspace {
		t.Errorf("a contact a human created is %q, want workspace", got)
	}
}

// A capture path still keeps its contact to the mailbox owner, and it does so
// by SAYING so — the spec carries OwnerScoped, so the privacy survives an
// actor-type rule going away.
func TestACapturedContactIsStillTheMailboxOwnersAlone(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.as(e.owner, principal.RowScopeAll)

	var id ids.PersonID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		owner := ids.From[ids.UserKind](e.owner)
		id, err = createPerson(ctx, tx, PersonResolution{Decision: DecisionNoMatch}, PersonSpec{
			FullName: "Unjudged Sender",
			// An owner-scoped row names its owner, or it is readable by nobody
			// at all — the database refuses the pair outright.
			Visibility: visibilityFor(true),
			OwnerID:    &owner,
			Source:     "capture",
			CapturedBy: "connector:gmail",
		})
		return err
	}); err != nil {
		t.Fatalf("minting an owner-scoped capture contact: %v", err)
	}

	if got := e.visibilityOf(t, id); got != visibilityOwner {
		t.Errorf("a capture-minted contact is %q, want owner: a mailbox with a year of history "+
			"names correspondents the workspace has no business reading", got)
	}
}

// asAgent is `as` with the principal a tool call arrives under: an agent acting
// on one human's behalf, carrying that human's own grants.
func (e *privacyEnv) asAgent(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:tools", UserID: user, OnBehalfOf: user,
		TeamIDs: []ids.UUID{e.team},
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"person": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}
