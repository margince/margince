// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The email drawer names the shared mailbox a message came in through, and the
// team that reads it. Only a reader of the message is told.
func TestTheEmailAccessNamesTheSharedMailboxAndItsReaderTeam(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	_, syncShared := sharedMailbox(t, env, e.Team1, capturemod.PostureHeld)
	syncShared(t, mailTo("shared-access-1@acme.example", "Where is my order?", sharedMailboxAddress))
	id := ids.From[ids.ActivityKind](oneActivityID(t, e))
	store := activities.NewStore(e.DB())

	member, err := store.GetEmailPresentation(readerAs(e, e.Rep1, e.Team1), id, nil)
	if err != nil {
		t.Fatalf("a reader-team member's email read: %v", err)
	}
	want := crmcontracts.EmailSharedMailbox{
		Address: sharedMailboxAddress, ReaderTeamId: openapi_types.UUID(e.Team1), ReaderTeamName: e.Team1.String(),
	}
	if got := member.Access.SharedMailboxes; got == nil || len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("shared_mailboxes = %+v, want exactly %+v", got, want)
	}
	if member.Access.Explanation == nil || *member.Access.Explanation != "shared_mailbox" {
		t.Errorf("explanation = %v, want shared_mailbox beside the named mailbox", member.Access.Explanation)
	}

	withheld, err := store.GetEmailPresentation(readerAs(e, e.Rep3, e.Team2), id, nil)
	if err == nil && withheld.Access.SharedMailboxes != nil {
		t.Fatalf("a seat outside the reader team was told the shared mailbox: %+v", *withheld.Access.SharedMailboxes)
	}
}

// Mail only personal mailboxes captured names no shared mailbox. The list is
// empty rather than absent, because the reader may read it and the answer is
// "none".
func TestPersonalMailNamesNoSharedMailbox(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	env.sync(t, mailTo("personal-access-1@acme.example", "Hello.", captureOwner))
	id := ids.From[ids.ActivityKind](oneActivityID(t, e))

	shown, err := activities.NewStore(e.DB()).GetEmailPresentation(readerCtx(e, e.Rep1), id, nil)
	if err != nil {
		t.Fatalf("the owner's email read: %v", err)
	}
	if got := shown.Access.SharedMailboxes; got == nil || len(*got) != 0 {
		t.Fatalf("shared_mailboxes = %v for personal mail, want an empty list", got)
	}
}
