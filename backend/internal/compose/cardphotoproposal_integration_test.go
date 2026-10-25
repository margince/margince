// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A photo of business cards against a real database. The cards become
// approvals that only the importer reads. No contact exists before a decision.
// Approving one creates that card's contact through the real writer.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func photoCards() []contacts.VCardEntry {
	return []contacts.VCardEntry{
		{
			FullName: "Hanna Fotograf", Company: "Blende GmbH", ModelRead: true,
			Emails: []contacts.VCardChannel{{Value: "hanna@blende.example", Kind: "work"}},
		},
		{
			FullName: "Paul Objektiv", Company: "Blende GmbH", ModelRead: true,
			Emails: []contacts.VCardChannel{{Value: "paul@blende.example", Kind: "work"}},
		},
	}
}

func countContactsWithEmail(ctx context.Context, t *testing.T, e *integration.Env, email string) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(ctx,
		`SELECT count(*) FROM contact_email WHERE lower(email) = $1`, email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPhotoCardsBecomeApprovalsAndCreateNothingUntilApproved(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	entries := photoCards()

	results, err := e.Contacts.ProposeCardsFromPhoto(ctx, entries)
	if err != nil {
		t.Fatal(err)
	}
	stage := vcardCreateStager(e.Pool)
	for _, r := range results {
		if !r.Outcome.AwaitsApproval() {
			t.Fatalf("card %d outcome = %s, want one that awaits approval", r.Index, r.Outcome)
		}
		if err := stage(ctx, entries[r.Index], r.ContactID); err != nil {
			t.Fatalf("staging card %d: %v", r.Index, err)
		}
	}
	for _, email := range []string{"hanna@blende.example", "paul@blende.example"} {
		if n := countContactsWithEmail(ctx, t, e, email); n != 0 {
			t.Fatalf("%s exists before any approval", email)
		}
	}

	svc := approvalsServiceWithEffects(e.Pool)
	status, kind := "pending", vcardCreateKind
	rows, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("pending approvals = %d, want one per card", len(rows))
	}
	for _, row := range rows {
		if row.Summary == nil || !strings.Contains(*row.Summary, "photo") {
			t.Errorf("summary %v does not say the card was read from a photo", row.Summary)
		}
	}
	colleague := e.As(e.Rep1, nil, integration.AdminPerms)
	if seen, _, err := svc.ListWire(colleague, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10}); err != nil || len(seen) != 0 {
		t.Errorf("a colleague read %d of the importer's photo cards (err %v), want none", len(seen), err)
	}

	if _, err := svc.Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id)), true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}
	created := countContactsWithEmail(ctx, t, e, "hanna@blende.example") + countContactsWithEmail(ctx, t, e, "paul@blende.example")
	if created != 1 {
		t.Errorf("contacts created after approving one of two cards = %d, want 1", created)
	}
}

// stagePhotoCard proposes one model-read card and returns its pending approval.
func stagePhotoCard(ctx context.Context, t *testing.T, e *integration.Env, entry contacts.VCardEntry) crmcontracts.Approval {
	t.Helper()
	results, err := e.Contacts.ProposeCardsFromPhoto(ctx, []contacts.VCardEntry{entry})
	if err != nil || len(results) != 1 || !results[0].Outcome.AwaitsApproval() {
		t.Fatalf("proposing the card: results %+v, err %v", results, err)
	}
	if err := vcardCreateStager(e.Pool)(ctx, entry, results[0].ContactID); err != nil {
		t.Fatal(err)
	}
	status, kind := "pending", vcardCreateKind
	rows, _, err := approvalsServiceWithEffects(e.Pool).ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("pending approvals = %d (err %v), want the one card", len(rows), err)
	}
	return rows[0]
}

func TestPhotoCardsShowTheNameTheApprovalWrites(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	row := stagePhotoCard(ctx, t, e, contacts.VCardEntry{
		FullName: "  Hanna FOTOGRAF ", Company: "Blende GmbH", ModelRead: true,
		Emails: []contacts.VCardChannel{{Value: "hanna@blende.example", Kind: "work"}},
	})

	shown, _ := (*row.ProposedChange)["name"].(string)
	if shown != "Hanna FOTOGRAF" {
		t.Fatalf("the approval shows %q, want the name as the create writes it", shown)
	}
	if _, err := approvalsServiceWithEffects(e.Pool).Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(row.Id)), true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}
	var written string
	if err := e.Pool.QueryRow(ctx, `SELECT c.full_name FROM contact c JOIN contact_email m ON m.contact_id = c.id
		WHERE m.email = 'hanna@blende.example'`).Scan(&written); err != nil {
		t.Fatal(err)
	}
	if written != shown {
		t.Errorf("the contact was written as %q after the approval showed %q", written, shown)
	}
}

// A card is proposed while its address is free. A colleague then files that
// address, and the approval must not create a second contact on top of it.
func TestPhotoCardsApprovedAfterTheAddressWasTakenCreateNothing(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	row := stagePhotoCard(ctx, t, e, photoCards()[0])

	colleague := e.As(e.Rep1, nil, integration.AdminPerms)
	if _, err := e.Contacts.CreateContact(colleague, contacts.CreateContactInput{
		FullName: "Hanna Fotograf", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "hanna@blende.example", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("the colleague's contact: %v", err)
	}

	_, err := approvalsServiceWithEffects(e.Pool).Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(row.Id)), true, nil)
	if err == nil {
		t.Error("approving a card whose address a contact now holds reported success")
	}
	if n := countContactsWithEmail(ctx, t, e, "hanna@blende.example"); n != 1 {
		t.Errorf("contacts holding the address = %d, want only the colleague's", n)
	}
}
