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

// signerMail is the mail a signature is read from, and the contact who sent it.
type signerMail struct {
	domain  string // the sender's domain; acme-mail.example when empty
	spoofed bool   // the sender's domain did not vouch for the From
	// private seeds an owner-scoped contact and mail only its owner imported.
	private bool
	cc      string // another party on the mail
	address *crmcontracts.Address
}

// signedMail seeds a contact and one mail they wrote, and answers both.
func (e *dedupeEnv) signedMail(ctx context.Context, t *testing.T, name string, mail signerMail) (ids.ContactID, ids.UUID) {
	t.Helper()
	if mail.domain == "" {
		mail.domain = "acme-mail.example"
	}
	from := "sig-" + ids.NewV7().String() + "@" + mail.domain
	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: name, Source: "connector:gmail", Address: mail.address,
		Emails: []ContactEmailInput{{Email: from, EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	return contactID, e.mailFrom(ctx, t, contactID, from, mail)
}

// mailFrom seeds one more mail the contact wrote from address from, dated
// after every mail seeded before it.
func (e *dedupeEnv) mailFrom(ctx context.Context, t *testing.T, contactID ids.ContactID, from string, mail signerMail) ids.UUID {
	t.Helper()
	activityID := ids.NewV7()
	audience := map[bool]string{false: "workspace", true: "participants"}[mail.private]
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO activity (id, kind, body, direction, occurred_at, source, captured_by, audience,
			inbound_sender_authenticated) VALUES ($1, 'email', $2, 'inbound', clock_timestamp(), 'gmail:seed', 'connector:gmail', $3, $4)`,
			activityID, signedBlock, audience, !mail.spoofed); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
			activityID, contactID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, contact_id, address, role) VALUES ($1, $2, $3, 'from')`,
			activityID, contactID, from); err != nil {
			return err
		}
		if mail.cc != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'cc')`,
				activityID, mail.cc); err != nil {
				return err
			}
		}
		if !mail.private {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, contactID, e.rep); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, activityID, e.rep)
		return err
	}); err != nil {
		t.Fatalf("seed the signed mail: %v", err)
	}
	return activityID
}

func (e *dedupeEnv) applySigned(ctx context.Context, t *testing.T, contactID ids.ContactID, activityID ids.UUID, fields ...SignatureField) {
	t.Helper()
	if _, err := e.store.ApplySignatureFields(ctx, contactID, activityID, fields, true); err != nil {
		t.Fatalf("applying the signature: %v", err)
	}
}

func signed(name, value string) SignatureField {
	return quoted(name, value, signedBlock)
}

func quoted(name, value, evidence string) SignatureField {
	return SignatureField{Name: name, Value: value, Evidence: evidence, Confidence: 0.95}
}

func TestASignatureFillsAnEmptyAddressAndNeverOneAColleagueDecided(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	filled := `SELECT count(*) FROM contact WHERE id = $1 AND address_line1 = 'Hafenstrasse 12'
	  AND address_postal_code = '20457' AND address_city = 'Hamburg' AND address_country = 'DE'`

	empty, mail := e.signedMail(ctx, t, "Dana Empty", signerMail{})
	e.applySigned(ctx, t, empty, mail, signed(fieldAddress, signedAddress))
	if got := e.count(ctx, t, filled, empty); got != 1 {
		t.Fatal("a signature's address line did not fill an empty address")
	}

	street := "Somewhere 1"
	typed, mail := e.signedMail(ctx, t, "Dana Typed", signerMail{address: &crmcontracts.Address{Line1: &street}})
	cleared, clearedMail := e.signedMail(ctx, t, "Dana Cleared", signerMail{})
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
	contactID, mail := e.signedMail(ctx, t, "Dana Second", signerMail{})
	other, _ := e.signedMail(ctx, t, "Olga Other", signerMail{})
	if _, err := e.store.UpdateContact(ctx, other, UpdateContactInput{Emails: []ContactEmailInput{
		{Email: "olga@acme-mail.example", EmailType: emailTypeWork, IsPrimary: true},
	}}); err != nil {
		t.Fatalf("giving the other contact an address: %v", err)
	}
	listing := signedBlock + " | olga@acme-mail.example | noreply@acme-mail.example"
	e.applySigned(ctx, t, contactID, mail,
		quoted(fieldEmail, "Dana.Weber@acme-mail.example", listing),
		quoted(fieldEmail, "olga@acme-mail.example", listing),
		quoted(fieldEmail, "noreply@acme-mail.example", listing))

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

func TestASignaturesEmailLandsOnlyFromItsAuthenticatedSendersOwnDomain(t *testing.T) {
	const stated = "dana.weber@acme-mail.example"
	for name, tc := range map[string]struct {
		mail  signerMail
		field SignatureField
		own   bool
	}{
		"a spoofed sender":                  {mail: signerMail{spoofed: true}, field: signed(fieldEmail, stated)},
		"an address on another domain":      {field: quoted(fieldEmail, "dana@elsewhere.example", signedBlock+" | dana@elsewhere.example")},
		"an address the line does not show": {field: signed(fieldEmail, "dana.other@acme-mail.example")},
		"another party's address":           {mail: signerMail{cc: stated}, field: signed(fieldEmail, stated)},
		"the workspace's own domain":        {field: signed(fieldEmail, stated), own: true},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			if tc.own {
				e.employee(ctx, t, nil, job{company: "Our Own Company", domain: "acme-mail.example", own: true})
			}
			contactID, mail := e.signedMail(ctx, t, "Dana Guarded", tc.mail)
			e.applySigned(ctx, t, contactID, mail, tc.field)
			if got := e.count(ctx, t, `SELECT count(*) FROM contact_email WHERE contact_id = $1 AND NOT is_primary`, contactID); got != 0 {
				t.Fatalf("with %s the signature added %d addresses to the contact", name, got)
			}
		})
	}
}

func TestANarrowedSignatureTakesBackItsAddressAndEmail(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, mail := e.signedMail(ctx, t, "Dana Narrowed", signerMail{})
	employer := e.employee(ctx, t, &contactID, job{company: "Acme Logistics", domain: "acme-mail.example"})
	e.applySigned(ctx, t, contactID, mail, signed(fieldAddress, signedAddress), signed(fieldEmail, "dana.weber@acme-mail.example"),
		signed(fieldCompanyName, "Acme Logistics"))
	if got := e.count(ctx, t, `SELECT count(*) FROM company WHERE id = $1 AND address_city = 'Hamburg'`, employer); got != 1 {
		t.Fatal("the employer's street was not filled before the narrowing")
	}
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
	if got := e.count(ctx, t, `SELECT count(*) FROM company WHERE id = $1 AND address_line1 IS NULL AND address_city IS NULL`, employer); got != 1 {
		t.Error("the employer's street read off a narrowed mail stayed on the company")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
	  WHERE a.entity_type = 'company' AND a.entity_id = $1 AND a.evidence->>'source' = 'fill_retracted'`, employer); got != 1 {
		t.Errorf("taking back the street wrote %d company audit rows with their event, want 1", got)
	}
}

// job is a company to seed and the contact's employment there.
type job struct {
	company, domain string
	// secondary seeds a current employment that does not hold the primary slot.
	secondary bool
	// own makes the company the workspace's own.
	own bool
}

// employee seeds a company on its domain, and the contact's current
// employment there when contactID is set.
func (e *dedupeEnv) employee(ctx context.Context, t *testing.T, contactID *ids.ContactID, j job) ids.CompanyID {
	t.Helper()
	created, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: j.company, Source: "manual",
		Domains: []CompanyDomainInput{{Domain: j.domain, IsPrimary: true}}})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(created.Id))
	if j.own {
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE company SET is_anchor = true WHERE id = $1`, companyID)
			return err
		}); err != nil {
			t.Fatalf("making the company the workspace's own: %v", err)
		}
	}
	if contactID == nil {
		return companyID
	}
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: "employment", ContactID: contactID, CompanyID: &companyID, IsCurrentPrimary: new(!j.secondary), Source: "manual",
	}); err != nil {
		t.Fatalf("seed employment: %v", err)
	}
	return companyID
}

func TestASignatureFillsItsOneEmployersEmptyStreet(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	street := `SELECT count(*) FROM company WHERE id = $1 AND address_line1 = 'Hafenstrasse 12' AND address_city = 'Hamburg'`

	contactID, mail := e.signedMail(ctx, t, "Dana Employed", signerMail{domain: "acme-logistics.example"})
	employer := e.employee(ctx, t, &contactID, job{company: "Acme Logistics", domain: "acme-logistics.example"})
	e.applySigned(ctx, t, contactID, mail, signed(fieldAddress, signedAddress), signed(fieldCompanyName, "Acme Logistics"))
	if got := e.count(ctx, t, street, employer); got != 1 {
		t.Fatal("the signature did not fill its employer's empty street")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
	  WHERE a.entity_type = 'company' AND a.entity_id = $1 AND a.evidence->>'source' = 'capture_enrich'`, employer); got != 1 {
		t.Errorf("the company fill wrote %d audit rows with their event, want 1", got)
	}

	untouched := map[string]ids.CompanyID{}
	refused := func(what string, mail signerMail, company, named string, before func(ids.ContactID, ids.CompanyID)) {
		signer, source := e.signedMail(ctx, t, "Dana "+company, mail)
		domain := mail.domain
		if company == "Acme Pier" {
			domain = "acme-pier.example"
		}
		untouched[what] = e.employee(ctx, t, &signer, job{company: company, domain: domain})
		if before != nil {
			before(signer, untouched[what])
		}
		e.applySigned(ctx, t, signer, source, signed(fieldAddress, signedAddress), signed(fieldCompanyName, named))
	}
	refused("two current employers", signerMail{domain: "acme-freight.example"}, "Acme Freight", "Acme Freight",
		func(signer ids.ContactID, _ ids.CompanyID) {
			e.employee(ctx, t, &signer, job{company: "Other Works", domain: "other-works.example", secondary: true})
		})
	refused("an employer the signature does not name", signerMail{domain: "acme-shipping.example"}, "Acme Shipping", "Somebody Else", nil)
	refused("a street a colleague cleared", signerMail{domain: "acme-docks.example"}, "Acme Docks", "Acme Docks",
		func(_ ids.ContactID, company ids.CompanyID) {
			city := "Bremen"
			if _, err := e.store.UpdateCompany(ctx, company, UpdateCompanyInput{Address: &crmcontracts.Address{City: &city}}); err != nil {
				t.Fatalf("typing the company's address: %v", err)
			}
			if _, err := e.store.UpdateCompany(ctx, company, UpdateCompanyInput{Address: &crmcontracts.Address{}}); err != nil {
				t.Fatalf("clearing the company's address: %v", err)
			}
		})
	refused("a spoofed sender", signerMail{domain: "acme-port.example", spoofed: true}, "Acme Port", "Acme Port", nil)
	refused("mail not open to the workspace", signerMail{domain: "acme-quay.example", private: true}, "Acme Quay", "Acme Quay", nil)
	refused("a sender off the company's domains", signerMail{domain: "pier-staff.example"}, "Acme Pier", "Acme Pier", nil)
	for what, company := range untouched {
		if got := e.count(ctx, t, `SELECT count(*) FROM company WHERE id = $1 AND address_line1 IS NULL`, company); got != 1 {
			t.Errorf("with %s the company's street was filled", what)
		}
	}
}

// senderOf is the contact's primary address, the one its seeded mail is from.
func (e *dedupeEnv) senderOf(ctx context.Context, t *testing.T, contactID ids.ContactID) string {
	t.Helper()
	var from string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT email FROM contact_email WHERE contact_id = $1 AND is_primary`, contactID).Scan(&from)
	}); err != nil {
		t.Fatalf("reading the sender: %v", err)
	}
	return from
}

// narrow limits each mail to its participants and takes back what it stated.
func (e *dedupeEnv) narrow(ctx context.Context, t *testing.T, mails ...ids.UUID) {
	t.Helper()
	for _, mail := range mails {
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE activity SET audience = 'participants' WHERE id = $1`, mail); err != nil {
				return err
			}
			return RetractSignatureFieldsTx(ctx, tx, mail)
		}); err != nil {
			t.Fatalf("narrowing a mail: %v", err)
		}
	}
}

func TestAValueTheSnippetDoesNotPrintWritesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, mail := e.signedMail(ctx, t, "Dana Injected", signerMail{domain: "acme-logistics.example"})
	employer := e.employee(ctx, t, &contactID, job{company: "Acme Logistics", domain: "acme-logistics.example"})
	e.applySigned(ctx, t, contactID, mail,
		signed(fieldAddress, "Evil Road 1, 10115 Berlin"),
		signed(fieldEmail, "attacker@acme-logistics.example"),
		quoted(fieldEmail, "victim@acme-logistics.example", "Dana Weber | notvictim@acme-logistics.example"),
		signed(fieldCompanyName, "Acme Logistics"))

	if got := e.count(ctx, t, `SELECT count(*) FROM contact WHERE id = $1 AND address_line1 IS NULL`, contactID); got != 1 {
		t.Error("an address the snippet does not print filled the contact's address")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM contact_profile_field WHERE contact_id = $1 AND field IN ('address', 'email')`, contactID); got != 0 {
		t.Errorf("%d evidence rows for values the snippet does not print, want none", got)
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM contact_email WHERE contact_id = $1 AND NOT is_primary`, contactID); got != 0 {
		t.Error("an address the snippet does not print, or prints only inside a longer one, became the contact's email")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM company WHERE id = $1 AND address_line1 IS NULL`, employer); got != 1 {
		t.Error("a street the snippet does not print filled the employer's address")
	}
}

func TestASignatureValueStaysWhileAnotherMailStillStatesIt(t *testing.T) {
	street := `SELECT count(*) FROM company WHERE id = $1 AND address_city = 'Hamburg'`
	email := `SELECT count(*) FROM contact_email WHERE contact_id = $1 AND email = 'dana.weber@acme-mail.example' AND archived_at IS NULL`
	address := `SELECT count(*) FROM contact WHERE id = $1 AND address_city = 'Hamburg'`
	for name, tc := range map[string]struct {
		narrowFirst, narrowSecond bool
		want                      int
	}{
		"narrowing only the first mail keeps the values":  {narrowFirst: true, want: 1},
		"narrowing only the second mail keeps the values": {narrowSecond: true, want: 1},
		"narrowing both mails takes the values back":      {narrowFirst: true, narrowSecond: true, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			contactID, first := e.signedMail(ctx, t, "Dana Twice", signerMail{})
			employer := e.employee(ctx, t, &contactID, job{company: "Acme Logistics", domain: "acme-mail.example"})
			statement := []SignatureField{signed(fieldAddress, signedAddress), signed(fieldEmail, "dana.weber@acme-mail.example"),
				signed(fieldCompanyName, "Acme Logistics")}
			e.applySigned(ctx, t, contactID, first, statement...)
			second := e.mailFrom(ctx, t, contactID, e.senderOf(ctx, t, contactID), signerMail{})
			e.applySigned(ctx, t, contactID, second, statement...)
			if tc.narrowFirst {
				e.narrow(ctx, t, first)
			}
			if tc.narrowSecond {
				e.narrow(ctx, t, second)
			}
			for what, query := range map[string]string{"street": street, "email": email, "address": address} {
				subject := contactID.UUID
				if what == "street" {
					subject = employer.UUID
				}
				if got := e.count(ctx, t, query, subject); got != tc.want {
					t.Errorf("the %s is present %d times, want %d", what, got, tc.want)
				}
			}
		})
	}
}

func TestANarrowingKeepsAnAddressAColleagueSetAgain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, mail := e.signedMail(ctx, t, "Dana Decided", signerMail{})
	e.applySigned(ctx, t, contactID, mail, signed(fieldAddress, signedAddress))
	other, signedStreet := "Elsewhere 2", "Hafenstrasse 12"
	city, code, country := "Hamburg", "20457", "DE"
	for _, line := range []*string{&other, &signedStreet} {
		if _, err := e.store.UpdateContact(ctx, contactID, UpdateContactInput{Address: &crmcontracts.Address{
			Line1: line, City: &city, PostalCode: &code, Country: &country,
		}}); err != nil {
			t.Fatalf("setting the address: %v", err)
		}
	}
	e.narrow(ctx, t, mail)

	if got := e.count(ctx, t, `SELECT count(*) FROM contact WHERE id = $1 AND address_line1 = 'Hafenstrasse 12'`, contactID); got != 1 {
		t.Fatal("narrowing the mail took back an address a colleague set after the fill")
	}
}

func TestTheStreetGoesWithTheLastMailThatVouchedForIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, vouched := e.signedMail(ctx, t, "Dana Vouched", signerMail{})
	employer := e.employee(ctx, t, &contactID, job{company: "Acme Logistics", domain: "acme-mail.example"})
	statement := []SignatureField{signed(fieldAddress, signedAddress), signed(fieldCompanyName, "Acme Logistics")}
	e.applySigned(ctx, t, contactID, vouched, statement...)
	// A spoofed mail may restate the contact's address, never the employer's street.
	spoofed := e.mailFrom(ctx, t, contactID, e.senderOf(ctx, t, contactID), signerMail{spoofed: true})
	e.applySigned(ctx, t, contactID, spoofed, statement...)
	e.narrow(ctx, t, vouched)

	if got := e.count(ctx, t, `SELECT count(*) FROM contact WHERE id = $1 AND address_city = 'Hamburg'`, contactID); got != 1 {
		t.Error("the contact's address went although another mail still states it")
	}
	if got := e.count(ctx, t, `SELECT count(*) FROM company WHERE id = $1 AND address_line1 IS NULL`, employer); got != 1 {
		t.Error("the employer's street stayed after the last mail that vouched for it was narrowed")
	}
}
