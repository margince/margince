// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Who can see a contact capture minted, and when that changes.
//
// Connecting a mailbox with a year of history names every correspondent in it —
// a lawyer, a doctor, a school. One email is not a reason to put any of them in
// front of every colleague, so a contact the SINK mints is the mailbox owner's
// until something judges their sender a business counterparty. The verdict path
// is that something, and it is the only ensure caller that asks for the
// workspace.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// contactVisibility answers how a captured contact is scoped. It fails when
// there is no such contact: a test that meant to find one and silently read the
// empty string would pass for the wrong reason.
func contactVisibility(t *testing.T, e *integration.SearchEnv, email string) string {
	t.Helper()
	var visibility string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT p.visibility FROM contact p
			  JOIN contact_email pe ON pe.contact_id = p.id
			 WHERE pe.email = $1 AND p.archived_at IS NULL`, email).Scan(&visibility)
	}); err != nil {
		t.Fatalf("reading the visibility of %s: %v", email, err)
	}
	return visibility
}

// contactOwnerOf answers whose the contact is.
func contactOwnerOf(t *testing.T, e *integration.SearchEnv, email string) ids.UUID {
	t.Helper()
	var owner *ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT p.owner_id FROM contact p
			  JOIN contact_email pe ON pe.contact_id = p.id
			 WHERE pe.email = $1 AND p.archived_at IS NULL`, email).Scan(&owner)
	}); err != nil {
		t.Fatalf("reading the owner of %s: %v", email, err)
	}
	if owner == nil {
		return ids.Nil
	}
	return *owner
}

func TestAContactTheSinkMintsBelongsToTheMailboxOwnerAlone(t *testing.T) {
	env := newCaptureEnv(t)
	e, syncSent := env.e, env.syncSent

	// The owner wrote to this address, which is T1 correspondence-positive and
	// mints the contact on the spot. Nothing has judged the sender: the evidence
	// is that somebody wrote to them, not that they are a business contact.
	const counterparty = "anwalt@kanzlei.example"
	syncSent(t, map[string]bool{"scope-1@kanzlei.example": true},
		emailSaying(counterparty, "scope-1@kanzlei.example", "", "können wir nächste Woche sprechen"))
	// They answer, which is the exchange the create tier needs. The subject here
	// is WHOSE the minted contact is, so the reply is fixture rather than
	// finding.
	env.sync(t, email(counterparty, "Anwalt", captureOwner, "scope-1r@kanzlei.example", "scope-1@kanzlei.example"))

	if got := contactVisibility(t, e, counterparty); got != "owner" {
		t.Fatalf("a contact the sink minted is %q, want owner — a year of history would otherwise publish every correspondent", got)
	}
	if got := contactOwnerOf(t, e, counterparty); got != e.Rep1 {
		t.Fatalf("the contact belongs to %s, want the mailbox owner %s", got, e.Rep1)
	}
}

func TestAVerdictPromotesTheContactItJudged(t *testing.T) {
	env := newCaptureEnv(t)
	e, syncSent := env.e, env.syncSent

	const counterparty = "einkauf@kunde.example"
	syncSent(t, map[string]bool{"promote-1@kunde.example": true},
		emailSaying(counterparty, "promote-1@kunde.example", "", "danke für das Angebot"))
	// They answer, which is the exchange the create tier needs. The subject here
	// is what a VERDICT then does to the record's visibility.
	env.sync(t, email(counterparty, "Einkauf", captureOwner, "promote-1r@kunde.example", "promote-1@kunde.example"))
	if got := contactVisibility(t, e, counterparty); got != "owner" {
		t.Fatalf("the contact starts %q, want owner", got)
	}

	// The verdict path ensures the same address WITHOUT asking for owner scope.
	// That is the decision: something judged this sender a business
	// counterparty, which is the moment the record becomes the workspace's.
	promoteByVerdict(t, e, counterparty, activityIDOf(t, e, "promote-1@kunde.example"))
	if got := contactVisibility(t, e, counterparty); got != "workspace" {
		t.Fatalf("a judged counterparty is still %q, want workspace", got)
	}

	// And the sink running again over a later message does NOT narrow it back.
	// It runs on every message that sender ever writes, so narrowing here would
	// un-publish the contact the next time they wrote.
	syncSent(t, map[string]bool{"promote-2@kunde.example": true},
		emailSaying(counterparty, "promote-2@kunde.example", "", "noch eine Frage"))
	if got := contactVisibility(t, e, counterparty); got != "workspace" {
		t.Fatalf("a later capture narrowed a promoted contact back to %q", got)
	}
}

// promoteByVerdict ensures the counterparty the way the verdict path does —
// workspace-scoped, because something judged the sender.
func promoteByVerdict(t *testing.T, e *integration.SearchEnv, email string, activityID ids.UUID) {
	t.Helper()
	store := contacts.NewStore(e.DB())
	// The grants the verdict engine runs with: it mints and links records, and
	// the promotion rides the same ensure.
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true},
				"activity": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, domain, _ := strings.Cut(email, "@")
		_, err := store.EnsureCounterpartyTx(ctx, tx, contacts.EnsureCounterpartyInput{
			Email:      email,
			Domain:     domain,
			ActivityID: ids.From[ids.ActivityKind](activityID),
			OwnerID:    e.Rep1,
			Source:     "verdict",
			CapturedBy: "agent:capture_counterparty_verdict",
		})
		return err
	}); err != nil {
		t.Fatalf("promoting by verdict: %v", err)
	}
}
