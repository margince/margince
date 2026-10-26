// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The per-field report of an import, read off the real writers. Only the
// database knows whether a statement replaced, restated or lost to the record.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reportRow finds the one row a report holds for a field, failing when the
// report holds none or several.
func reportRow(t *testing.T, fields []VCardField, field string) VCardField {
	t.Helper()
	var found []VCardField
	for _, f := range fields {
		if f.Field == field {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		t.Fatalf("report rows for %s = %+v, want one; the whole report: %+v", field, found, fields)
	}
	return found[0]
}

func TestACardWithAWorkNumberAndNoMobileReportsTheMobileAsNotInTheCard(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Wanda Work\nORG:Desk Phones AG\nURL:https://desk-phones.example\n"+
			"EMAIL;TYPE=WORK:wanda@desk-phones.example\nTEL;TYPE=WORK:+49 30 1234567\nEND:VCARD\n")
	if len(results) != 1 || results[0].Outcome != VCardCreated {
		t.Fatalf("outcomes = %+v, want one created", results)
	}
	fields := results[0].Fields

	if got := reportRow(t, fields, fieldPhone); got.Action != VCardFieldImported || got.Value != "+49301234567" {
		t.Errorf("phone = %+v, want the work number imported in E.164", got)
	}
	if got := reportRow(t, fields, reportFieldMobilePhone); got.Action != VCardFieldNotInCard || got.Value != "" {
		t.Errorf("mobile = %+v, want not_in_card with no value", got)
	}
	for field, want := range map[string]VCardFieldAction{
		reportFieldName: VCardFieldImported, reportFieldEmail: VCardFieldImported,
		fieldCompanyName: VCardFieldImported, fieldWebsite: VCardFieldImported,
		fieldTitle: VCardFieldNotInCard, fieldAddress: VCardFieldNotInCard, fieldLinkedin: VCardFieldNotInCard,
	} {
		if got := reportRow(t, fields, field); got.Action != want {
			t.Errorf("%s = %+v, want %s", field, got, want)
		}
	}
	// The website the report names is on the record, filed as evidence.
	if got := profileFieldValue(ctx, t, e, *results[0].ContactID, fieldWebsite); got != "https://desk-phones.example" {
		t.Errorf("website on the record = %q, want the card's", got)
	}
}

func TestAMatchedCardThatReplacesATitleReportsItReplaced(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	typed := "Chief Financial Officer"
	incumbent, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Rhea Replace", Source: "manual", Title: &typed,
		Emails: []ContactEmailInput{{Email: "rhea@replace.example", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}

	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Rhea Replace\nTITLE:VP Finance\n"+
			"EMAIL;TYPE=WORK:rhea@replace.example\nEMAIL;TYPE=HOME:rhea@home.example\nEND:VCARD\n")
	if len(results) != 1 || results[0].Outcome != VCardUpdated {
		t.Fatalf("outcomes = %+v, want one updated", results)
	}
	fields := results[0].Fields
	if got := reportRow(t, fields, fieldTitle); got.Action != VCardFieldReplaced || got.Value != "VP Finance" {
		t.Errorf("title = %+v, want the card's title as replaced", got)
	}
	if got := reportRow(t, fields, reportFieldName); got.Action != VCardFieldKept {
		t.Errorf("name = %+v, want kept: a card never renames a matched contact", got)
	}
	emails := map[string]VCardFieldAction{}
	for _, f := range fields {
		if f.Field == reportFieldEmail {
			emails[f.Value] = f.Action
		}
	}
	if emails["rhea@replace.example"] != VCardFieldImported || emails["rhea@home.example"] != VCardFieldKept {
		t.Errorf("emails = %v, want the held address imported and the new one kept", emails)
	}

	// The same card again restates the title; it neither replaces nor loses.
	again := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Rhea Replace\nTITLE:VP Finance\nEMAIL;TYPE=WORK:rhea@replace.example\nEND:VCARD\n")
	if got := reportRow(t, again[0].Fields, fieldTitle); got.Action != VCardFieldImported {
		t.Errorf("restated title = %+v, want imported", got)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(incumbent.Id))
	if got := supersededFieldValue(ctx, t, e, contactID, fieldTitle); got != typed {
		t.Errorf("superseded title = %q, want the typed %q the report said it replaced", got, typed)
	}
}

// A correction on the ledger needs the verdict module's ruling, so a store
// built without that reader refuses the card rather than guessing.
func TestACardOverACorrectionNeedsTheVerdictReader(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Cora Corrected", Source: "manual",
		Emails: []ContactEmailInput{{Email: "cora@corrected.example", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}
	key := sha256.Sum256([]byte("profile_field:" + fieldTitle))
	e.execAs(ctx, t, `INSERT INTO ai_feedback
		(subject_type, subject_id, claim_kind, claim_key, verdict, corrected_value, source, captured_by)
		VALUES ('contact', $1, 'profile_field', $2, 'corrected', 'Head of Treasury', 'human', 'human:x')`,
		incumbent.Id, hex.EncodeToString(key[:]))

	entries, err := ParseVCards(strings.NewReader(
		"BEGIN:VCARD\nFN:Cora Corrected\nTITLE:VP Finance\nEMAIL;TYPE=WORK:cora@corrected.example\nEND:VCARD\n"))
	if err != nil {
		t.Fatalf("parsing the card: %v", err)
	}
	if _, err := e.store.ImportVCards(ctx, entries); !errors.Is(err, errNoProfileVerdictReader) {
		t.Errorf("import = %v, want the refusal for a store with no verdict reader", err)
	}
}

func TestACardWithTwoWorkNumbersReportsBoth(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if _, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Tom Two", Source: "manual",
		Emails: []ContactEmailInput{{Email: "tom@two.example", EmailType: emailTypeWork, IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}
	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Tom Two\nEMAIL;TYPE=WORK:tom@two.example\n"+
			"TEL;TYPE=WORK:+49 30 1111111\nTEL;TYPE=WORK:+49 30 2222222\nEND:VCARD\n")
	if len(results) != 1 || results[0].Outcome != VCardUpdated {
		t.Fatalf("outcomes = %+v, want one updated", results)
	}
	phones := map[string]VCardFieldAction{}
	for _, f := range results[0].Fields {
		if f.Field == fieldPhone {
			phones[f.Value] = f.Action
		}
	}
	if len(phones) != 2 || phones["+49301111111"] != VCardFieldImported || phones["+49302222222"] != VCardFieldImported {
		t.Errorf("phones = %v, want both work numbers imported", phones)
	}
}

func TestAPhotoProposalAndAReviewCarryNoFieldReport(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if _, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Rory Review", Source: "manual",
		Emails: []ContactEmailInput{{Email: "rory@first.example", EmailType: emailTypeWork, IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}
	review := importCards(ctx, t, e, "BEGIN:VCARD\nFN:Rory Review\nEMAIL:rory@second.example\nEND:VCARD\n")
	if review[0].Outcome != VCardNeedsReview || review[0].Fields != nil {
		t.Errorf("review = %+v, want needs_review with no field report", review[0])
	}
	photo, err := e.store.ProposeCardsFromPhoto(ctx, []VCardEntry{{FullName: "Pia Photo", ModelRead: true}})
	if err != nil {
		t.Fatalf("proposing from a photo: %v", err)
	}
	if len(photo) != 1 || photo[0].Outcome != VCardProposed || photo[0].Fields != nil {
		t.Errorf("photo = %+v, want one proposed with no field report", photo)
	}
}
