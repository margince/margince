// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// What a patched edge records.
//
// An edge is where a person's job title, their start and leaving dates and
// their current employer live, and a patch is the only writer of any of them.
// The audit row is the one place a later reader can learn that a role was
// rewritten or an employment ended — the statement itself reports only that
// something moved.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asEdgeEditor is the rep holding the grant a PATCH takes. as() carries
// relationship create and read; editing an edge takes relationship.update as
// well, and the anchor's update grant it already has.
func (e *dedupeEnv) asEdgeEditor(t *testing.T) context.Context {
	t.Helper()
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the rep context carries no actor")
	}
	grant := actor.Permissions.Objects["relationship"]
	grant.Update = true
	actor.Permissions.Objects["relationship"] = grant
	return principal.WithActor(ctx, actor)
}

// A promotion and a leaving date land in one patch, and the row says what each
// field held. The kind moved with neither of them, so it is not presented as a
// change nobody made.
func TestAPatchedEdgeRecordsTheRoleAndDatesItReplaced(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.asEdgeEditor(t)
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	former, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Kessler Werke", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed the earlier employer: %v", err)
	}
	formerID := ids.From[ids.OrganizationKind](ids.UUID(former.Id))
	analyst := "Analyst"
	edge, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: "employment", PersonID: &personID, OrganizationID: &formerID,
		Role: &analyst, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed the earlier employment: %v", err)
	}

	promoted := "Head of Analytics"
	left := time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC)
	if _, err := e.store.UpdateRelationship(ctx, edge.ID, UpdateRelationshipInput{
		Role: &promoted, EndedAt: &left,
	}); err != nil {
		t.Fatalf("patch the employment: %v", err)
	}

	before, after := auditImagesHolding(ctx, t, e.store, "relationship", edge.ID, relationshipRoleField)
	wantImage(t, before, "before", relationshipRoleField, analyst)
	wantImage(t, after, "after", relationshipRoleField, promoted)
	wantImage(t, before, "before", "ended_at", nil)
	if after["ended_at"] == nil {
		t.Errorf("after[ended_at] = nil, want the date the job ended: %v", after)
	}
	if _, moved := after[relationshipKindField]; moved {
		t.Errorf("the after image presents the kind as a change, and a patch cannot move it: %v", after)
	}
}

// The other half: a patch that ends a CURRENT primary employment clears the
// flag in the statement, and the images are what say the person stopped
// working there — nothing else on the record does.
func TestAPatchedEdgeRecordsTheCurrentEmployerFlagItCleared(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.asEdgeEditor(t)
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Terralogic", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed the employer: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Jonas Brede", Source: "manual",
		Emails: []PersonEmailInput{{Email: "jonas@terralogic.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seed the person: %v", err)
	}
	personID := ids.From[ids.PersonKind](ids.UUID(person.Id))

	primary := true
	edge, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: "employment", PersonID: &personID, OrganizationID: &orgID,
		IsCurrentPrimary: &primary, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed the current employment: %v", err)
	}
	if !edge.IsCurrentPrimary {
		t.Fatalf("the seeded employment is not the current primary one: %+v", edge)
	}

	left := time.Date(2025, time.March, 31, 0, 0, 0, 0, time.UTC)
	patched, err := e.store.UpdateRelationship(ctx, edge.ID, UpdateRelationshipInput{EndedAt: &left})
	if err != nil {
		t.Fatalf("end the employment: %v", err)
	}
	if patched.IsCurrentPrimary {
		t.Fatal("an employment that is over is still marked as the current primary one")
	}

	before, after := auditImagesHolding(ctx, t, e.store, "relationship", edge.ID, "is_current_primary")
	wantImage(t, before, "before", "is_current_primary", true)
	wantImage(t, after, "after", "is_current_primary", false)
	wantImage(t, before, "before", "ended_at", nil)
}
