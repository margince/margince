// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A contact its owner removed from the capture waiting list stays removed when
// the same correspondent writes again.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const removedCorrespondent = "probe@demo-removed.example"

// ownerDeskCtx is the mailbox owner in the app, with the contact grants the
// waiting list's remove action needs.
func ownerDeskCtx(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(),
		UserID: e.Rep1, TeamIDs: []ids.UUID{e.Team1}, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"contact": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func waitingFor(t *testing.T, e *integration.SearchEnv) []capturemod.AwaitingContact {
	t.Helper()
	listed, err := capturemod.ContactsAwaitingDecisionFor(ownerDeskCtx(e), compose.InstallationDB(e.Pool))
	if err != nil {
		t.Fatalf("reading the waiting list: %v", err)
	}
	return listed
}

func TestAContactRemovedFromTheWaitingListDoesNotReturnOnTheNextMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	// The owner's attested reply is the send that lets capture mint the contact.
	env.syncSent(t, map[string]bool{"rm2@myco.example": true},
		email(removedCorrespondent, "Probe Test", captureOwner, "rm1@demo-removed.example", ""),
		email(captureOwner, "", removedCorrespondent, "rm2@myco.example", "rm1@demo-removed.example"),
		email(removedCorrespondent, "Probe Test", captureOwner, "rm3@demo-removed.example", "rm1@demo-removed.example"),
	)
	listed := waitingFor(t, e)
	if len(listed) != 1 {
		t.Fatalf("the waiting list holds %d contacts after the exchange, want the one capture minted", len(listed))
	}

	// The list's remove action is DELETE /contacts/{id}, which archives here.
	removed := ids.From[ids.ContactKind](listed[0].ContactID)
	if _, err := contacts.NewStore(compose.InstallationDB(e.Pool)).ArchiveContact(ownerDeskCtx(e), removed, nil); err != nil {
		t.Fatalf("removing the contact from the waiting list: %v", err)
	}

	env.sync(t, email(removedCorrespondent, "Probe Test", captureOwner, "rm4@demo-removed.example", "rm1@demo-removed.example"))

	if again := waitingFor(t, e); len(again) != 0 {
		t.Fatalf("the removed correspondent is back on the waiting list as %d contact(s)", len(again))
	}
	if n := countRows(t, e, `SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = '`+removedCorrespondent+`' AND p.archived_at IS NULL`); n != 0 {
		t.Fatalf("%d live contacts for the removed correspondent, want none", n)
	}
}
