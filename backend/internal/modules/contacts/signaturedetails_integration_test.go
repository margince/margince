// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A signature's postal address, further email and employer's street reach the
// record, filling only what is empty and nobody decided.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	signedAddress = "Hafenstrasse 12, 20457 Hamburg, Deutschland"
	signedBlock   = "Dana Weber | Acme Logistics | Hafenstrasse 12, 20457 Hamburg, Deutschland | dana.weber@acme-mail.example"
)

// signedMail seeds a contact and one open mail they wrote, and answers both.
func (e *dedupeEnv) signedMail(ctx context.Context, t *testing.T, name string) (ids.ContactID, ids.UUID) {
	t.Helper()
	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: name, Source: "connector:gmail",
		Emails: []ContactEmailInput{{Email: "sig-" + ids.NewV7().String() + "@seed.test", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	activityID := ids.NewV7()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO activity (id, kind, body, direction, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', $2, 'inbound', now(), 'gmail:seed', 'connector:gmail', 'workspace')`, activityID, signedBlock)
		return err
	}); err != nil {
		t.Fatalf("seed the signed mail: %v", err)
	}
	e.writtenBy(ctx, t, contactID, activityID)
	return contactID, activityID
}

func (e *dedupeEnv) applySigned(ctx context.Context, t *testing.T, contactID ids.ContactID, activityID ids.UUID, fields ...SignatureField) {
	t.Helper()
	if _, err := e.store.ApplySignatureFields(ctx, contactID, activityID, fields, true); err != nil {
		t.Fatalf("applying the signature: %v", err)
	}
}

func signed(name, value string) SignatureField {
	return SignatureField{Name: name, Value: value, Evidence: signedBlock, Confidence: 0.95}
}

func TestASignatureFillsAnEmptyAddressAndNeverOneAColleagueDecided(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	filled := `SELECT count(*) FROM contact WHERE id = $1 AND address_line1 = 'Hafenstrasse 12'
	  AND address_postal_code = '20457' AND address_city = 'Hamburg' AND address_country = 'DE'`

	empty, mail := e.signedMail(ctx, t, "Dana Empty")
	e.applySigned(ctx, t, empty, mail, signed(fieldAddress, signedAddress))
	if got := e.count(ctx, t, filled, empty); got != 1 {
		t.Fatal("a signature's address line did not fill an empty address")
	}

	typed, mail := e.signedMail(ctx, t, "Dana Typed")
	street := "Somewhere 1"
	if _, err := e.store.UpdateContact(ctx, typed, UpdateContactInput{Address: &crmcontracts.Address{Line1: &street}}); err != nil {
		t.Fatalf("typing an address: %v", err)
	}
	cleared, clearedMail := e.signedMail(ctx, t, "Dana Cleared")
	if _, err := e.store.UpdateContact(ctx, cleared, UpdateContactInput{Address: &crmcontracts.Address{Line1: &street}}); err != nil {
		t.Fatalf("typing an address: %v", err)
	}
	if _, err := e.store.UpdateContact(ctx, cleared, UpdateContactInput{Address: &crmcontracts.Address{}}); err != nil {
		t.Fatalf("clearing the address: %v", err)
	}
	e.applySigned(ctx, t, typed, mail, signed(fieldAddress, signedAddress))
	e.applySigned(ctx, t, cleared, clearedMail, signed(fieldAddress, signedAddress))
	if got := e.count(ctx, t, `SELECT count(*) FROM contact WHERE id = $1 AND address_line1 = 'Somewhere 1' AND address_city IS NULL`, typed); got != 1 {
		t.Error("a signature changed an address a colleague typed")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM contact WHERE id = $1 AND address_line1 IS NULL`, cleared); got != 1 {
		t.Error("a signature refilled an address a colleague cleared")
	}
}

func TestASignaturesFurtherEmailBecomesASecondAddressOfThatContactOnly(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, mail := e.signedMail(ctx, t, "Dana Second")
	other, _ := e.signedMail(ctx, t, "Olga Other")
	if _, err := e.store.UpdateContact(ctx, other, UpdateContactInput{Emails: []ContactEmailInput{
		{Email: "olga@acme-mail.example", EmailType: emailTypeWork, IsPrimary: true},
	}}); err != nil {
		t.Fatalf("giving the other contact an address: %v", err)
	}
	e.applySigned(ctx, t, contactID, mail,
		signed(fieldEmail, "Dana.Weber@acme-mail.example"),
		signed(fieldEmail, "olga@acme-mail.example"),
		signed(fieldEmail, "noreply@acme-mail.example"))

	if got := e.count(ctx, t, `SELECT count(*) FROM contact_email WHERE contact_id = $1 AND email = 'dana.weber@acme-mail.example'
	  AND NOT is_primary AND captured_by = 'agent:enrich' AND NOT from_correspondence AND archived_at IS NULL`, contactID); got != 1 {
		t.Fatal("the signature's own further address is not a second, machine-marked address of the contact")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM contact_email WHERE email = 'olga@acme-mail.example' AND archived_at IS NULL`); got != 1 {
		t.Error("an address another contact holds was copied onto the signer")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM contact_email WHERE email = 'noreply@acme-mail.example'`); got != 0 {
		t.Error("a sending system's address became a contact's email")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM contact_profile_field WHERE contact_id = $1 AND field = 'email'`, contactID); got != 1 {
		t.Errorf("%d email evidence rows, want the one that landed", got)
	}
}

func TestANarrowedSignatureTakesBackItsAddressAndEmail(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, mail := e.signedMail(ctx, t, "Dana Narrowed")
	e.applySigned(ctx, t, contactID, mail, signed(fieldAddress, signedAddress), signed(fieldEmail, "dana.weber@acme-mail.example"))
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE activity SET audience = 'participants' WHERE id = $1`, mail); err != nil {
			return err
		}
		return RetractSignatureFieldsTx(ctx, tx, mail)
	}); err != nil {
		t.Fatalf("narrowing the mail: %v", err)
	}

	if got := e.count(ctx, t, `SELECT count(*) FROM contact WHERE id = $1 AND address_line1 IS NULL AND address_city IS NULL`, contactID); got != 1 {
		t.Error("the address read off a narrowed mail stayed on the contact")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM contact_email WHERE contact_id = $1 AND email = 'dana.weber@acme-mail.example'
	  AND archived_at IS NULL`, contactID); got != 0 {
		t.Error("the email read off a narrowed mail stayed on the contact")
	}
}

// employee seeds a current employment of the contact at a new company.
func (e *dedupeEnv) employee(ctx context.Context, t *testing.T, contactID ids.ContactID, company string, primary bool) ids.CompanyID {
	t.Helper()
	created, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: company, Source: "manual"})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(created.Id))
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: new(primary), Source: "manual",
	}); err != nil {
		t.Fatalf("seed employment: %v", err)
	}
	return companyID
}

func TestASignatureFillsItsOneEmployersEmptyStreet(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	street := `SELECT count(*) FROM company WHERE id = $1 AND address_line1 = 'Hafenstrasse 12' AND address_city = 'Hamburg'`

	contactID, mail := e.signedMail(ctx, t, "Dana Employed")
	employer := e.employee(ctx, t, contactID, "Acme Logistics", true)
	e.applySigned(ctx, t, contactID, mail, signed(fieldAddress, signedAddress), signed(fieldCompanyName, "Acme Logistics"))
	if got := e.count(ctx, t, street, employer); got != 1 {
		t.Fatal("the signature did not fill its employer's empty street")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
	  WHERE a.entity_type = 'company' AND a.entity_id = $1 AND a.evidence->>'source' = 'capture_enrich'`, employer); got != 1 {
		t.Errorf("the company fill wrote %d audit rows with their event, want 1", got)
	}

	twoJobs, mail := e.signedMail(ctx, t, "Dana Two Jobs")
	first := e.employee(ctx, t, twoJobs, "Acme Freight", true)
	e.employee(ctx, t, twoJobs, "Other Works", false)
	e.applySigned(ctx, t, twoJobs, mail, signed(fieldAddress, signedAddress), signed(fieldCompanyName, "Acme Freight"))

	otherName, mail := e.signedMail(ctx, t, "Dana Elsewhere")
	unnamed := e.employee(ctx, t, otherName, "Acme Shipping", true)
	e.applySigned(ctx, t, otherName, mail, signed(fieldAddress, signedAddress), signed(fieldCompanyName, "Somebody Else"))
	for what, company := range map[string]ids.CompanyID{"two current employers": first, "an employer the signature does not name": unnamed} {
		if got := e.count(ctx, t, `SELECT count(*) FROM company WHERE id = $1 AND address_line1 IS NULL`, company); got != 1 {
			t.Errorf("with %s the company's street was filled", what)
		}
	}
}
