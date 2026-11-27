// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A contact its owner removed from the capture waiting list stays removed.
// Neither the next message nor the sender check's answer about older mail
// mints it again.

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

// removeTheOnlyWaitingContact presses the list's remove action, which is
// DELETE /contacts/{id} and archives here.
func removeTheOnlyWaitingContact(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	listed := waitingFor(t, e)
	if len(listed) != 1 {
		t.Fatalf("the waiting list holds %d contacts, want the one capture minted", len(listed))
	}
	removed := ids.From[ids.ContactKind](listed[0].ContactID)
	if _, err := contacts.NewStore(compose.InstallationDB(e.Pool)).ArchiveContact(ownerDeskCtx(e), removed, nil); err != nil {
		t.Fatalf("removing the contact from the waiting list: %v", err)
	}
}

// answerEverySenderAsAContact runs the sender check with a model that calls
// every sender a contact.
func answerEverySenderAsAContact(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	engine := compose.NewCounterpartyVerdictEngine(e.Pool, contactVerdict{}, compose.CaptureConfig{}, quietLog())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
}

func requireNoLiveContact(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	if again := waitingFor(t, e); len(again) != 0 {
		t.Fatalf("the removed correspondent is back on the waiting list as %d contact(s)", len(again))
	}
	if n := countRows(t, e, `SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = '`+removedCorrespondent+`' AND p.archived_at IS NULL`); n != 0 {
		t.Fatalf("%d live contacts for the removed correspondent, want none", n)
	}
}

// syncExchange lands a stranger's message, the owner's attested reply, and
// their answer. The reply is the send that lets capture mint the contact; the
// first message opens a sender question.
func syncExchange(t *testing.T, env captureEnv) {
	t.Helper()
	env.syncSent(t, map[string]bool{"rm2@myco.example": true},
		email(removedCorrespondent, "Probe Test", captureOwner, "rm1@demo-removed.example", ""),
		email(captureOwner, "", removedCorrespondent, "rm2@myco.example", "rm1@demo-removed.example"),
		email(removedCorrespondent, "Probe Test", captureOwner, "rm3@demo-removed.example", "rm1@demo-removed.example"),
	)
}

func TestAContactRemovedFromTheWaitingListDoesNotReturnOnTheNextMessage(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	syncExchange(t, env)
	removeTheOnlyWaitingContact(t, env.e)

	env.sync(t, email(removedCorrespondent, "Probe Test", captureOwner, "rm4@demo-removed.example", "rm1@demo-removed.example"))

	requireNoLiveContact(t, env.e)
}

// The sender question was open when the owner removed the contact. The
// machine's later answer is about mail from before the removal.
func TestTheSenderCheckDoesNotMintAContactRemovedBeforeItAnswered(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	syncExchange(t, env)
	removeTheOnlyWaitingContact(t, env.e)

	answerEverySenderAsAContact(t, env.e)

	requireNoLiveContact(t, env.e)
	if n := countRows(t, env.e, `SELECT count(*) FROM capture_pending_counterparty
		WHERE email = '`+removedCorrespondent+`' AND status = 'rejected'`); n != 1 {
		t.Fatalf("%d rejected answers for the removed correspondent, want 1: the ledger must settle the sender", n)
	}
}

// The sender check asked and could not tell, and the owner removed the contact.
// A provider recovery later reopens that question, and its new answer is still
// about mail from before the removal.
func TestTheSenderCheckDoesNotMintAContactRemovedAfterItCouldNotTell(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	syncExchange(t, env)
	parkThroughAnOutage(t, env.e, capturemod.NewPendingStore(env.e.DB()))
	if got := ledgerOf(t, env.e, removedCorrespondent).status; got != capturemod.PendingStatusUnsure {
		t.Fatalf("the parked question is %q, want unsure: the case is not set up", got)
	}
	removeTheOnlyWaitingContact(t, env.e)

	if _, err := compose.NewProviderRecovery(env.e.Pool).Reopen(context.Background(), outageWindow(), 0); err != nil {
		t.Fatalf("reopening the parked question: %v", err)
	}
	answerEverySenderAsAContact(t, env.e)

	requireNoLiveContact(t, env.e)
}
