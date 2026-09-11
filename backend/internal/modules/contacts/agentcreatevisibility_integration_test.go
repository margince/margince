// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Who can see a contact an AGENT typed in.
//
// A create through the tool surface reaches the same store method the UI and
// the REST API reach, so the only thing that ever distinguished them was the
// principal behind the call. It used to: an agent-created contact was born
// owner-scoped and no colleague could see it, which reads to the contact who
// asked for it as the contact simply not being there.
//
// The capture paths are the ones that still mint owner-scoped contacts, and
// they say so on the spec rather than being inferred from who is acting.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAContactAnAgentCreatesBelongsToTheWorkspace(t *testing.T) {
	e := setupCapturePrivacy(t)

	created, err := e.store.CreateContact(e.asAgent(e.owner), CreateContactInput{
		FullName: "Lucy Vo",
		Emails:   []ContactEmailInput{{Email: "lucy.vo@kunde.example", EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("creating a contact as an agent: %v", err)
	}

	if got := e.visibilityOf(t, ids.From[ids.ContactKind](ids.UUID(created.Id))); got != visibilityWorkspace {
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

	created, err := e.store.CreateContact(e.as(e.owner, principal.RowScopeAll), CreateContactInput{
		FullName: "Marta Kern",
		Emails:   []ContactEmailInput{{Email: "marta.kern@kunde.example", EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("creating a contact as a human: %v", err)
	}

	if got := e.visibilityOf(t, ids.From[ids.ContactKind](ids.UUID(created.Id))); got != visibilityWorkspace {
		t.Errorf("a contact a human created is %q, want workspace", got)
	}
}

// Storage is not the claim; being findable is. A row stored `workspace` that
// a colleague's read still filtered out would satisfy the assertions above and
// leave the reported symptom exactly where it was — the teammate goes looking
// and finds nothing.
//
// The capture side is deliberately NOT re-asserted here. It is covered end to
// end by TestAnAdvisorVerdictMakesTheRecordAndKeepsItTheOwnersAlone, which
// drives the real verdict engine; a second version calling createContact with a
// hand-written spec would pass whatever the production wiring did.
func TestAColleagueCanReadTheContactAnAgentCreated(t *testing.T) {
	e := setupCapturePrivacy(t)

	created, err := e.store.CreateContact(e.asAgent(e.owner), CreateContactInput{
		FullName: "Lucy Vo",
		Emails:   []ContactEmailInput{{Email: "lucy.read@kunde.example", EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("creating a contact as an agent: %v", err)
	}

	id := ids.From[ids.ContactKind](ids.UUID(created.Id))
	got, err := e.store.GetContact(e.as(e.teammate, principal.RowScopeOwn), id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("a colleague reading the contact an agent created: %v — the rep was told it "+
			"exists and their teammate cannot reach it", err)
	}
	if got.FullName != "Lucy Vo" {
		t.Errorf("the colleague read %q, want Lucy Vo", got.FullName)
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
				"contact": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}
