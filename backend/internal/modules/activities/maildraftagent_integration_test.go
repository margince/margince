// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// An agent's draft, against a real database: it lands in the saved drafts of
// the human the agent acts for, marked, under that human's row scope, and never
// over words the human saved.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asAgentFor binds an agent acting for the rep: the rep's grants and scope,
// the way identity renders a passport principal.
func (e *sendEnv) asAgentFor(t *testing.T, scope principal.RowScope) context.Context {
	t.Helper()
	human, ok := principal.Actor(e.as(scope))
	if !ok {
		t.Fatal("the rep's context carries no principal")
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(),
		UserID: e.rep, OnBehalfOf: e.rep, PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeDraft), Permissions: human.Permissions,
	})
}

func agentDraft(body string) MailDraftContent {
	return MailDraftContent{To: []string{"Buyer@Example.test"}, Subject: "After the demo", Body: body}
}

func TestAnAgentsDraftWaitsForItsHumanMarkedAsAnAgents(t *testing.T) {
	e := setupSend(t)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	store := draftStore(e)

	saved, err := store.SaveAgentMailDraft(e.asAgentFor(t, principal.RowScopeAll), anchor, agentDraft("First go"), nil)
	if err != nil {
		t.Fatalf("an agent saving a draft: %v", err)
	}
	got, err := store.GetMailDraft(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("the human reading the agent's draft: %v", err)
	}
	if got.ID != saved.ID || !got.AgentDrafted || got.Content.Body != "First go" || got.Content.To[0] != "buyer@example.test" {
		t.Fatalf("the human read %+v, want the agent's draft, marked, with its address canonical", got)
	}

	again, err := store.SaveAgentMailDraft(e.asAgentFor(t, principal.RowScopeAll), anchor, agentDraft("Shorter"), nil)
	if err != nil || again.ID != saved.ID || again.Version != 2 || again.Content.Body != "Shorter" {
		t.Fatalf("a second agent draft = %+v (%v), want the waiting draft replaced at v2", again, err)
	}
	if actions, _ := e.draftTrail(t, saved.ID); len(actions) != 2 || actions[0] != "create" || actions[1] != "update" {
		t.Errorf("the trail records %v, want create then update", actions)
	}
	var activities int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM activity WHERE subject = 'After the demo'`).Scan(&activities); err != nil || activities != 0 {
		t.Errorf("an agent's draft wrote %d activities (%v); a draft is never on the timeline", activities, err)
	}
}

func TestTheHumansSaveClearsTheMarkAndAnAgentCannotWriteOverIt(t *testing.T) {
	e := setupSend(t)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	store := draftStore(e)
	human := e.as(principal.RowScopeAll)

	saved, err := store.SaveAgentMailDraft(e.asAgentFor(t, principal.RowScopeAll), anchor, agentDraft("The agent's words"), nil)
	if err != nil {
		t.Fatalf("an agent saving a draft: %v", err)
	}
	edited := agentDraft("My words now")
	kept, err := store.SaveMailDraft(human, anchor, edited, &saved.Version)
	if err != nil || kept.AgentDrafted {
		t.Fatalf("the human's save = %+v (%v), want the mark cleared", kept, err)
	}
	if _, err := store.SaveAgentMailDraft(e.asAgentFor(t, principal.RowScopeAll), anchor, agentDraft("Overwrite"), nil); !errors.Is(err, ErrOwnDraftWaiting) {
		t.Errorf("an agent writing over the human's draft → %v, want ErrOwnDraftWaiting", err)
	}
	got, err := store.GetMailDraft(human, anchor)
	if err != nil || got.Content.Body != "My words now" {
		t.Errorf("the human's draft now reads %q (%v), want their words untouched", got.Content.Body, err)
	}
}

func TestAnAgentsDraftAnswersToItsHumansRowScopeAndOnlyAnAgentMarksOne(t *testing.T) {
	e := setupSend(t)
	hidden := replyAnchor(e.seedAnchor(t, "", ""))
	e.linkToContactOwnedBy(t, ids.From[ids.ActivityKind](hidden.ID), e.other)
	store := draftStore(e)

	if _, err := store.SaveAgentMailDraft(e.asAgentFor(t, principal.RowScopeOwn), hidden, agentDraft("x"), nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an agent drafting on a record its human cannot see → %v, want ErrNotFound", err)
	}
	visible := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	if _, err := store.SaveAgentMailDraft(e.as(principal.RowScopeAll), visible, agentDraft("x"), nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a human writing an agent-marked draft → %v, want ErrPermissionDenied", err)
	}
}
