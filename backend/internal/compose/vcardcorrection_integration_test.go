// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A human's correction against an imported card, through the wired store and
// the verdict module's own ruling. A correction stands only while it still
// describes the value the record holds.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedCorrectedTitle makes a contact whose title row holds shown, and records
// a human's correction of that value through the ledger's own writer.
func seedCorrectedTitle(ctx context.Context, t *testing.T, e *integration.Env, email, shown string) ids.ContactID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
		FullName: "Cora Corrected", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	saveTitleClaim(ctx, t, e, contactID, shown)
	corrected := "Head of Treasury"
	if err := ai.NewFeedbackStore(InstallationDB(e.Pool)).Record(ctx, ai.RecordInput{
		SubjectType: "contact", SubjectID: contactID.UUID,
		ClaimKind: ai.ClaimProfileField, ClaimPath: ai.ProfileFieldClaimPath("title"),
		Verdict: ai.VerdictCorrected, CorrectedValue: &corrected, ValueShown: &shown,
	}); err != nil {
		t.Fatalf("recording the correction: %v", err)
	}
	return contactID
}

// saveTitleClaim stores a title as an accepted research claim.
func saveTitleClaim(ctx context.Context, t *testing.T, e *integration.Env, contactID ids.ContactID, title string) {
	t.Helper()
	count, err := e.Contacts.SaveResearchClaims(ctx, contactID, []contacts.ResearchClaimInput{{
		Field: "title", Value: title, Quote: title, SourceURL: "https://source.example.test/team",
	}})
	if err != nil || count != 1 {
		t.Fatalf("saving the title claim: %d, %v", count, err)
	}
}

// importOneCard imports a card and returns its one result.
func importOneCard(ctx context.Context, t *testing.T, e *integration.Env, card contacts.VCardEntry) contacts.VCardResult {
	t.Helper()
	results, err := e.Contacts.ImportVCards(ctx, []contacts.VCardEntry{card})
	if err != nil {
		t.Fatalf("importing the card: %v", err)
	}
	if len(results) != 1 || results[0].Outcome != contacts.VCardUpdated {
		t.Fatalf("outcomes = %+v, want one updated", results)
	}
	return results[0]
}

// fieldAction is the one action a report gives a field.
func fieldAction(t *testing.T, result contacts.VCardResult, field string) contacts.VCardFieldAction {
	t.Helper()
	var found []contacts.VCardFieldAction
	for _, f := range result.Fields {
		if f.Field == field {
			found = append(found, f.Action)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s rows = %v, want one; the report: %+v", field, found, result.Fields)
	}
	return found[0]
}

func TestACorrectedTitleIsKeptAndTheCardsWebsiteStillLands(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	contactID := seedCorrectedTitle(ctx, t, e, "cora@fresh.example", "Engineer")

	result := importOneCard(ctx, t, e, contacts.VCardEntry{
		FullName: "Cora Corrected", Title: "VP Finance", URL: "https://fresh.example",
		Emails: []contacts.VCardChannel{{Value: "cora@fresh.example", Kind: "work"}},
	})
	if got := fieldAction(t, result, "title"); got != contacts.VCardFieldKept {
		t.Errorf("title = %s, want kept: the correction describes the value the record holds", got)
	}
	if got := fieldAction(t, result, "website"); got != contacts.VCardFieldImported {
		t.Errorf("website = %s, want imported: a correction of the title says nothing about the website", got)
	}
	after, err := e.Contacts.GetContact(ctx, contactID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the contact: %v", err)
	}
	if after.Title != nil && *after.Title == "VP Finance" {
		t.Errorf("title = %q, want the card's title not written over a standing correction", *after.Title)
	}
}

func TestACorrectionOfAValueSinceReplacedNoLongerStopsACard(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	contactID := seedCorrectedTitle(ctx, t, e, "cora@stale.example", "Engineer")
	// An accepted claim replaces the value the human corrected, so the
	// correction is about a value the record no longer holds.
	saveTitleClaim(ctx, t, e, contactID, "Director")

	result := importOneCard(ctx, t, e, contacts.VCardEntry{
		FullName: "Cora Corrected", Title: "VP Finance",
		Emails: []contacts.VCardChannel{{Value: "cora@stale.example", Kind: "work"}},
	})
	if got := fieldAction(t, result, "title"); got != contacts.VCardFieldReplaced {
		t.Errorf("title = %s, want replaced: the correction no longer describes the stored title", got)
	}
	after, err := e.Contacts.GetContact(ctx, contactID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the contact: %v", err)
	}
	if after.Title == nil || *after.Title != "VP Finance" {
		t.Errorf("title = %v, want the card's newer title applied", after.Title)
	}
}
