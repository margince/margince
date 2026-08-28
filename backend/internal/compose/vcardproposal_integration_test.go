// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The vCard near-match review loop against a real database: the import
// refuses the near-match, the refusal becomes ONE durable proposal however
// often the same card is uploaded, approving it creates the person through
// the real writer, and a decline is remembered against the card's identity.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The existing contact's address and the card's: SAME name, different
// address. An exact email match is merged outright; the near-match the
// import refuses to create is the name collision with no shared key.
const (
	existingContactEmail = "anna.weber@example.com"
	reviewedCardEmail    = "a.weber@webers.example"
)

// reviewedCard is the card that collides on the name with a person the
// workspace already holds — the shape the import refuses to create.
func reviewedCard() people.VCardEntry {
	return people.VCardEntry{
		FullName:     "Anna Weber",
		Organization: "Weber Consulting",
		Emails:       []people.VCardChannel{{Value: reviewedCardEmail, Kind: "work"}},
	}
}

// seedNearMatch writes the existing contact through the real writer and
// answers the import's review verdict for the card that resembles them.
func seedNearMatch(ctx context.Context, t *testing.T, e *integration.Env) *ids.PersonID {
	t.Helper()
	if _, err := e.People.CreatePerson(ctx, people.CreatePersonInput{
		FullName: "Anna Weber", Source: "ui",
		Emails: []people.PersonEmailInput{{Email: existingContactEmail, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding the existing contact: %v", err)
	}
	results, err := e.People.ImportVCards(ctx, []people.VCardEntry{reviewedCard()})
	if err != nil {
		t.Fatalf("importing the near-match card: %v", err)
	}
	if len(results) != 1 || results[0].Outcome != people.VCardNeedsReview {
		t.Fatalf("import outcome = %+v, want the one needs_review refusal", results)
	}
	return results[0].PersonID
}

func TestAVCardNearMatchBecomesOneDurableProposal(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	candidate := seedNearMatch(ctx, t, e)
	if candidate == nil {
		t.Fatal("the import named no visible candidate for an admin importer")
	}

	stage := vcardCreateStager(e.Pool)
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("staging the review: %v", err)
	}
	// The same card again — a re-uploaded file — joins the pending question
	// rather than asking it twice.
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("re-staging the review: %v", err)
	}

	svc := approvalsServiceWithEffects(e.Pool)
	status := "pending"
	kind := vcardCreateKind
	rows, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the staged reviews: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("pending vcard_create proposals = %d, want the one joined question", len(rows))
	}
	if rows[0].TargetEntityId == nil || ids.UUID(*rows[0].TargetEntityId) != candidate.UUID {
		t.Errorf("the proposal targets %v, want the near-match candidate", rows[0].TargetEntityId)
	}

	if _, err := svc.Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id)), true, nil); err != nil {
		t.Fatalf("approving the create: %v", err)
	}
	var created int
	if err := e.Pool.QueryRow(ctx,
		`SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		  WHERE lower(pe.email) = $1`, reviewedCardEmail).Scan(&created); err != nil {
		t.Fatalf("counting people holding the card's address: %v", err)
	}
	if created != 1 {
		t.Errorf("people holding the card's own address %s = %d, want exactly the approved create", reviewedCardEmail, created)
	}
}

func TestADeclinedVCardReviewIsNotReAsked(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	candidate := seedNearMatch(ctx, t, e)

	stage := vcardCreateStager(e.Pool)
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("staging the review: %v", err)
	}
	svc := approvalsServiceWithEffects(e.Pool)
	status, kind := "pending", vcardCreateKind
	rows, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("staged reviews = %d (err %v), want 1", len(rows), err)
	}
	if _, err := svc.Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id)), false, nil); err != nil {
		t.Fatalf("declining the create: %v", err)
	}

	// The same card in tomorrow's upload: the decline is remembered against
	// the card's identity, and the question is not asked again.
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("re-staging after the decline: %v", err)
	}
	rows, _, err = svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("re-listing: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("a declined card was re-proposed: %d pending rows", len(rows))
	}
}
