// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// draft_email's first message over real Postgres: the draft lands in the saved
// drafts of the human the agent acts for, and never over a draft of theirs.

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// draftingAgentFor is the passport principal identity renders for the human,
// holding the draft scope draft_email is admitted on.
func draftingAgentFor(t *testing.T, e *integration.Env, human ids.UUID) context.Context {
	t.Helper()
	ctx := e.AgentFor(t, human, nil, integration.AdminPerms)
	agent, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the agent context carries no principal")
	}
	agent.Scopes = principal.NewScopeSet(principal.ScopeRead, principal.ScopeDraft)
	return principal.WithActor(ctx, agent)
}

func TestDraftEmailLeavesTheDraftForItsHumanAndNeverOverTheirs(t *testing.T) {
	e := integration.Setup(t)
	contactID := e.SeedContact(t, "Dana Buyer", nil)
	address := openapi_types.Email("dana@example.test")
	view := oneContactView{contact: crmcontracts.Contact{
		Id: openapi_types.UUID(contactID), FullName: "Dana Buyer", PrimaryEmail: &address,
	}}
	store := activities.NewStore(e.DB())
	comms := commsAdapter{store: store, firstDrafts: &firstMessageEngines{contact: contactdraft.NewService(view, nil)}}
	links := []agents.RecordLink{{EntityType: "contact", EntityID: contactID}}
	anchor := activities.MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: contactID}
	human := e.As(e.Rep1, nil, integration.AdminPerms)

	first, err := comms.DraftCompanyEmail(draftingAgentFor(t, e, e.Rep1), links, "follow up on the demo")
	if err != nil {
		t.Fatalf("draft_email: %v", err)
	}
	if first.SavedDraftID == nil || first.NotSaved != "" || len(first.To) != 1 || first.To[0] != string(address) {
		t.Fatalf("draft_email answered %+v, want a saved draft addressed to the contact", first)
	}
	waiting, err := store.GetMailDraft(human, anchor)
	if err != nil || waiting.ID != *first.SavedDraftID || !waiting.AgentDrafted || waiting.Content.Body != first.Body {
		t.Fatalf("the human reads %+v (%v), want the agent's draft, marked", waiting, err)
	}

	if _, err := store.SaveMailDraft(human, anchor, activities.MailDraftContent{Body: "My own words"}, &waiting.Version); err != nil {
		t.Fatalf("the human saving over it: %v", err)
	}
	second, err := comms.DraftCompanyEmail(draftingAgentFor(t, e, e.Rep1), links, "shorter")
	if err != nil {
		t.Fatalf("draft_email over the human's draft: %v", err)
	}
	if second.SavedDraftID != nil || second.NotSaved == "" || second.Body == "" {
		t.Errorf("draft_email answered %+v, want the text returned unsaved with not_saved", second)
	}
	kept, err := store.GetMailDraft(human, anchor)
	if err != nil || kept.Content.Body != "My own words" {
		t.Errorf("the human's draft reads %q (%v), want their words untouched", kept.Content.Body, err)
	}
}
