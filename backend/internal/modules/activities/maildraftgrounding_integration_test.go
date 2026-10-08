// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// An agent's draft is served only while its reader may still see every record
// its words were written from. Once one is out of reach the draft is discarded
// on read, through the same audited delete a human's discard takes.

import (
	"context"
	"errors"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asArchiver is the rep holding the activity delete grant, so a test can
// archive a conversation through the real lifecycle verb.
func (e *sendEnv) asArchiver() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Delete: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// groundedDraft leaves an agent's draft on a contact, written from that contact
// and from one conversation, and answers the draft and the conversation.
func groundedDraft(t *testing.T, e *sendEnv, store *Store, extra ...MailDraftAnchor) (MailDraft, MailDraftAnchor) {
	t.Helper()
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	conversation := replyAnchor(e.seedAnchor(t, "", ""))
	grounding := append([]MailDraftAnchor{anchor, conversation}, extra...)
	saved, err := store.SaveAgentMailDraft(e.asAgentFor(t, principal.RowScopeAll), anchor, agentDraft("From what they said"), grounding)
	if err != nil {
		t.Fatalf("an agent saving a grounded draft: %v", err)
	}
	return saved, conversation
}

func TestAnAgentsDraftIsServedWhileEveryRecordItRestsOnIsVisible(t *testing.T) {
	e := setupSend(t)
	store := draftStore(e)
	saved, _ := groundedDraft(t, e, store)

	got, err := store.GetMailDraft(e.as(principal.RowScopeAll), saved.Anchor)
	if err != nil {
		t.Fatalf("reading a draft whose grounding is all visible: %v", err)
	}
	if got.ID != saved.ID || !slices.Equal(got.Grounding, saved.Grounding) || len(got.Grounding) != 2 {
		t.Errorf("read %+v, want the saved draft with its two grounding records", got)
	}
}

func TestAnAgentsDraftRestingOnAnArchivedConversationIsDiscardedOnRead(t *testing.T) {
	e := setupSend(t)
	store := draftStore(e)
	saved, conversation := groundedDraft(t, e, store)
	if _, err := store.ArchiveActivity(e.asArchiver(), ids.From[ids.ActivityKind](conversation.ID), nil); err != nil {
		t.Fatalf("archiving the conversation: %v", err)
	}

	if _, err := store.GetMailDraft(e.as(principal.RowScopeAll), saved.Anchor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading a draft written from an archived conversation → %v, want ErrNotFound", err)
	}
	if e.draftExists(t, saved.ID) {
		t.Error("the draft survived the read; it must be discarded, not merely hidden")
	}
	if actions, _ := e.draftTrail(t, saved.ID); !slices.Equal(actions, []string{"create", "delete"}) {
		t.Errorf("the trail records %v, want create then delete", actions)
	}
}

func TestAnAgentsDraftRestingOnARecordItsReaderHoldsNoGrantForIsDiscardedOnRead(t *testing.T) {
	e := setupSend(t)
	store := draftStore(e)
	project := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeProject, ID: e.seedProject(t, "ERP")}
	saved, _ := groundedDraft(t, e, store, project)

	// The rep's role holds no project grant, so the project the words drew on
	// is not theirs to read, whatever its row scope says.
	if _, err := store.GetMailDraft(e.as(principal.RowScopeAll), saved.Anchor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading a draft written from an ungranted project → %v, want ErrNotFound", err)
	}
	if e.draftExists(t, saved.ID) {
		t.Error("the draft survived the read; it must be discarded, not merely hidden")
	}
}

func TestTheHumansEditKeepsWhatTheAgentsDraftWasWrittenFrom(t *testing.T) {
	e := setupSend(t)
	store := draftStore(e)
	saved, conversation := groundedDraft(t, e, store)
	human := e.as(principal.RowScopeAll)
	if _, err := store.SaveMailDraft(human, saved.Anchor, agentDraft("My words now"), &saved.Version); err != nil {
		t.Fatalf("the human's edit: %v", err)
	}
	got, err := store.GetMailDraft(human, saved.Anchor)
	if err != nil || got.AgentDrafted || !slices.Equal(got.Grounding, saved.Grounding) {
		t.Fatalf("after the edit read %+v (%v), want the mark cleared and the grounding kept", got, err)
	}

	if _, err := store.ArchiveActivity(e.asArchiver(), ids.From[ids.ActivityKind](conversation.ID), nil); err != nil {
		t.Fatalf("archiving the conversation: %v", err)
	}
	if _, err := store.GetMailDraft(human, saved.Anchor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an edited draft written from an archived conversation → %v, want ErrNotFound", err)
	}
}
