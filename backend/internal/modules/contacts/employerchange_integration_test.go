// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Capture never moves an employer. When a contact's newest mail points at
// another company, the contact's owner is asked. The evidence must be a
// business domain or a signature naming an existing company. It must be newer
// than all that places the contact at the employer held.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// askedChanges records the questions a store would put to an owner.
type askedChanges struct {
	asked []approvalsubject.EmployerChange
}

func (a *askedChanges) ProposeEmployerChangeTx(_ context.Context, _ pgx.Tx, change approvalsubject.EmployerChange) error {
	a.asked = append(a.asked, change)
	return nil
}

// movedContact is Ann, employed at Oldco. She has addresses at Oldco, Newco,
// a domain nobody has settled yet and a consumer domain.
type movedContact struct {
	contact        ids.ContactID
	oldco, newco   ids.CompanyID
	employmentMade time.Time
}

func (e *dedupeEnv) askingStore() *askedChanges {
	asked := &askedChanges{}
	e.store = e.store.WithEmployerChangeProposer(asked)
	return asked
}

func (e *dedupeEnv) seedMovedContact(ctx context.Context, t *testing.T) movedContact {
	t.Helper()
	m := movedContact{
		oldco: e.createNamedCompany(ctx, t, "Oldco", "oldco.test"),
		newco: e.createNamedCompany(ctx, t, "Newco", "newco.test"),
	}
	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ann Lee", Source: "manual",
		Emails: []ContactEmailInput{
			{Email: "ann@oldco.test", EmailType: "work", IsPrimary: true},
			{Email: "ann@newco.test", EmailType: "work"},
			{Email: "ann@triaged.test", EmailType: "work"},
			{Email: "ann@gmail.com", EmailType: "personal"},
		},
	})
	if err != nil {
		t.Fatalf("seeding Ann: %v", err)
	}
	m.contact = ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if err := e.store.ResolveContactEmployer(ctx, m.contact, EmployerChoice{CompanyID: &m.oldco, Source: "manual"}); err != nil {
		t.Fatalf("employing Ann at Oldco: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT created_at FROM relationship WHERE contact_id = $1 AND kind = 'employment'`,
			m.contact).Scan(&m.employmentMade)
	}); err != nil {
		t.Fatal(err)
	}
	return m
}

// after is a mail time once the employment was recorded, inside the clock skew
// a header may show.
func (m movedContact) after(seconds int) time.Time {
	return m.employmentMade.Add(time.Duration(seconds) * time.Second)
}

func (e *dedupeEnv) ensureMail(ctx context.Context, t *testing.T, email, domain string, at time.Time) {
	t.Helper()
	if _, err := e.store.EnsureCounterparty(ctx, e.datedEnsureInput(ctx, t, email, domain, at)); err != nil {
		t.Fatalf("ensuring mail from %s: %v", email, err)
	}
}

func requireAsked(t *testing.T, asked *askedChanges, want int) {
	t.Helper()
	if len(asked.asked) != want {
		t.Fatalf("the owner was asked %d times, want %d: %+v", len(asked.asked), want, asked.asked)
	}
}

func TestMailFromANewBusinessDomainAsksTheOwner(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)

	e.ensureMail(ctx, t, "ann@newco.test", "newco.test", m.after(30))

	requireAsked(t, asked, 1)
	got := asked.asked[0]
	if got.OwnerID != e.rep || got.ContactID != m.contact.UUID || got.FromCompanyID != m.oldco.UUID ||
		got.ToCompanyID != m.newco.UUID || got.Domain != "newco.test" || got.ContactName != "Ann Lee" ||
		got.FromCompanyName != "Oldco" || got.ToCompanyName != "Newco" {
		t.Fatalf("asked %+v, want Ann's owner asked about Oldco to Newco from newco.test", got)
	}
	if employer := e.employerOf(ctx, t, m.contact); employer == nil || *employer != m.oldco.UUID {
		t.Fatalf("Ann's employer is %v after the question, want Oldco until her owner answers", employer)
	}
}

func TestAConsumerOrOwnDomainAsksNobody(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)
	// A company somebody filed on a consumer domain before capture refused one,
	// reached by the backlog sweep, which asks no consumer list of its own.
	fans := e.createNamedCompany(ctx, t, "Mail Fans")
	e.addLegacyDomain(ctx, t, fans, "gmail.com")
	e.ensureMail(ctx, t, "ann@gmail.com", "gmail.com", m.after(30))
	if _, err := e.store.AttachDomainBacklog(ctx, DomainBacklog{Domain: "gmail.com", CompanyID: fans}); err != nil {
		t.Fatalf("attaching the gmail.com backlog: %v", err)
	}
	requireAsked(t, asked, 0)

	e.claimOwnDomain(ctx, t, "newco.test")
	e.ensureMail(ctx, t, "ann@newco.test", "newco.test", m.after(60))
	requireAsked(t, asked, 0)
}

// claimOwnDomain makes a domain the workspace's own, as an administrator
// confirms one.
func (e *dedupeEnv) claimOwnDomain(ctx context.Context, t *testing.T, domain string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workspace_email_domain (domain, verified) VALUES ($1, true)`, domain)
		return err
	}); err != nil {
		t.Fatalf("claiming %s: %v", domain, err)
	}
}

// A backfill reads old mail after the record was set up, and a contact who
// still writes from the employer held has not left it.
func TestOlderMailOrNewerMailFromTheEmployerHeldAsksNobody(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)

	e.ensureMail(ctx, t, "ann@newco.test", "newco.test", m.employmentMade.AddDate(-1, 0, 0))
	requireAsked(t, asked, 0)

	e.datedEnsureInput(ctx, t, "ann@oldco.test", "oldco.test", m.after(90))
	e.ensureMail(ctx, t, "ann@newco.test", "newco.test", m.after(30))
	requireAsked(t, asked, 0)
}

// A record no member made has no owner, and nobody else is asked instead.
func TestAContactWithNoOwnerRaisesNothing(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	m := e.seedMovedContact(e.asNonHuman(principal.PrincipalSystem), t)
	e.ensureMail(e.as(), t, "ann@newco.test", "newco.test", m.after(30))
	requireAsked(t, asked, 0)
}

func TestALeadIsNotTouched(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	e.createNamedCompany(ctx, t, "Newco", "newco.test")
	lead := e.seedLead(ctx, t, "lena@newco.test", "lena@newco.test", "seed")
	res, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "lena@newco.test", "newco.test", time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC)))
	if err != nil || res.LeadID != lead {
		t.Fatalf("ensure = %+v (err %v), want the mail filed on the lead", res, err)
	}
	requireAsked(t, asked, 0)
	if n := e.countInWorkspace(ctx, t, `SELECT count(*) FROM relationship WHERE kind = 'employment'`); n != 0 {
		t.Fatalf("%d employments after mail to a lead, want none", n)
	}
}

// A domain question settled after the mail arrived asks about the contacts
// on it who already work somewhere else.
func TestASettledDomainAsksAboutContactsEmployedElsewhere(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)
	e.ensureMail(ctx, t, "ann@triaged.test", "triaged.test", m.after(30))
	requireAsked(t, asked, 0)

	verdict, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "triaged.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: e.startTriageRead(ctx, t, "triaged.test"),
		DossierName: "Triaged GmbH", SeedURL: "https://triaged.test",
	})
	if err != nil || verdict.CompanyID == nil {
		t.Fatalf("resolve = %+v (err %v), want a company", verdict, err)
	}
	requireAsked(t, asked, 1)
	if got := asked.asked[0]; got.ToCompanyID != verdict.CompanyID.UUID || got.FromCompanyID != m.oldco.UUID {
		t.Fatalf("asked %+v, want Oldco to the triaged company", got)
	}
}

func companyNameField(name string) SignatureField {
	return SignatureField{Name: fieldCompanyName, Value: name, Evidence: "Ann Lee | " + name, Confidence: 0.9}
}

// A signature names a company only by its name. So only the one company we
// hold under that name counts, and never the employer held.
func TestANewerSignatureNamingAnotherCompanyAsksTheOwner(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)

	for _, name := range []string{"Oldco", "Nobody Ltd"} {
		if _, err := e.store.ApplySignatureFields(ctx, m.contact, e.openSignatureSource(ctx, t, m.contact),
			[]SignatureField{companyNameField(name)}, true); err != nil {
			t.Fatalf("applying a signature naming %s: %v", name, err)
		}
	}
	requireAsked(t, asked, 0)

	if _, err := e.store.ApplySignatureFields(ctx, m.contact, e.openSignatureSource(ctx, t, m.contact),
		[]SignatureField{companyNameField("Newco")}, true); err != nil {
		t.Fatalf("applying a signature naming Newco: %v", err)
	}
	requireAsked(t, asked, 1)
	if got := asked.asked[0]; got.ToCompanyID != m.newco.UUID || got.Signature != "Ann Lee | Newco" || got.Domain != "" {
		t.Fatalf("asked %+v, want Newco with the signature line as evidence", got)
	}
}

func TestASignatureSentFromTheEmployerHeldAsksNobody(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)
	source := e.openSignatureSource(ctx, t, m.contact)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE activity_participant SET address = 'ann@oldco.test' WHERE activity_id = $1`, source)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ApplySignatureFields(ctx, m.contact, source, []SignatureField{companyNameField("Newco")}, true); err != nil {
		t.Fatalf("applying the signature: %v", err)
	}
	requireAsked(t, asked, 0)
}

// The anchor company is the workspace's own. A signature naming it is a
// colleague's, or somebody claiming to be one.
func TestASignatureNamingTheAnchorCompanyAsksNobody(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company SET is_anchor = true WHERE id = $1`, m.newco)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ApplySignatureFields(ctx, m.contact, e.openSignatureSource(ctx, t, m.contact),
		[]SignatureField{companyNameField("Newco")}, true); err != nil {
		t.Fatalf("applying the signature: %v", err)
	}
	requireAsked(t, asked, 0)
}

// A contact who holds a colleague's address is that colleague, whose employer
// is ours to know.
func TestAColleaguesContactAsksNobody(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE app_user SET email = 'ann@gmail.com' WHERE id = $1`, e.otherRep)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.ensureMail(ctx, t, "ann@newco.test", "newco.test", m.after(30))
	requireAsked(t, asked, 0)
}

// A held message is out of everybody's sight, so it is no evidence to show.
func TestAHeldMessageAsksNobody(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)
	var held ids.ActivityID
	if err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
		held, err = seedDomainEvidence(ctx, tx, domainEvidenceFixture{
			address: "ann@newco.test", at: m.after(30), precision: "instant", restricted: true, archived: true,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	mail := e.datedEnsureInput(ctx, t, "ann@newco.test", "newco.test", m.after(30))
	mail.ActivityID = held
	if _, err := e.store.EnsureCounterparty(ctx, mail); err != nil {
		t.Fatalf("ensuring the held mail: %v", err)
	}
	requireAsked(t, asked, 0)
}
