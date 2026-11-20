// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Anyone can send mail. Only the contact's own mail asks the owner about a new
// employer. It comes from an address of theirs on the company's domain, that
// domain vouched for it, and the owner may read it.

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// party is one participant of a captured message.
type party struct{ role, address string }

// sentMail captures one message dated after Ann's employment was recorded. It
// has the direction, sender check and parties the case names, and is filed for
// her Newco address.
func (e *dedupeEnv) sentMail(t *testing.T, m movedContact, direction string, verified bool, parties ...party) EnsureCounterpartyInput {
	t.Helper()
	ctx := e.as()
	activity := ids.New[ids.ActivityKind]()
	e.execTx(ctx, t, `
		INSERT INTO activity (id, kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
		                      inbound_sender_authenticated)
		VALUES ($1, 'email', 'hi', $2, $3, 'gmail', $4, 'gmail:seed', 'connector:gmail', $5)`,
		activity, direction, m.after(30), activity.String(), verified)
	for _, p := range parties {
		e.execTx(ctx, t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, $2, $3)`,
			activity, p.role, p.address)
	}
	return EnsureCounterpartyInput{
		Email: "ann@newco.test", Domain: "newco.test", OwnerID: e.rep, ActivityID: activity,
		Source: "gmail:" + activity.String(), CapturedBy: "connector:gmail", Replied: true,
	}
}

func TestOnlyTheContactsOwnVouchedForMailAsks(t *testing.T) {
	cases := []struct {
		name      string
		direction string
		verified  bool
		parties   []party
		want      int
	}{
		{"her own mail, vouched for", "inbound", true, []party{{"from", "ann@newco.test"}}, 1},
		{"a forged From", "inbound", false, []party{{"from", "ann@newco.test"}}, 0},
		{
			"copied on somebody else's mail", "inbound", true,
			[]party{{"from", "bob@newco.test"}, {"cc", "ann@newco.test"}},
			0,
		},
		{
			"a recipient of somebody else's mail", "inbound", true,
			[]party{{"from", "bob@newco.test"}, {"to", "ann@newco.test"}},
			0,
		},
		{
			"the rep's own mail to her", "outbound", true,
			[]party{{"from", "rep@ours.test"}, {"to", "ann@newco.test"}},
			0,
		},
		{"outbound mail that names her as From", "outbound", true, []party{{"from", "ann@newco.test"}}, 0},
		{"her mail from an address on another domain", "inbound", true, []party{{"from", "ann@gmail.com"}}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupDedupe(t)
			asked := e.askingStore()
			m := e.seedMovedContact(e.as(), t)
			e.seedBob(t)
			if _, err := e.store.EnsureCounterparty(e.as(), e.sentMail(t, m, tc.direction, tc.verified, tc.parties...)); err != nil {
				t.Fatalf("ensuring the mail: %v", err)
			}
			requireAsked(t, asked, tc.want)
			if tc.want == 1 && (asked.asked[0].FromAddress != "ann@newco.test" || asked.asked[0].SenderCheck != senderCheckAuthenticated) {
				t.Fatalf("asked %+v, want the From address and its sender check on the card", asked.asked[0])
			}
		})
	}
}

// seedBob is another contact, at Newco, whose mail may copy Ann.
func (e *dedupeEnv) seedBob(t *testing.T) {
	t.Helper()
	if _, err := e.store.CreateContact(e.as(), CreateContactInput{
		FullName: "Bob Ray", Source: "manual",
		Emails: []ContactEmailInput{{Email: "bob@newco.test", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding Bob: %v", err)
	}
}

// A signature from a personal address its domain did not vouch for is
// somebody writing in her name.
func TestAForgedSignatureAsksNobody(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)
	if _, err := e.store.ApplySignatureFields(ctx, m.contact, e.signatureFrom(ctx, t, m, "ann@gmail.com", false),
		[]SignatureField{companyNameField("Newco")}, true); err != nil {
		t.Fatalf("applying the signature: %v", err)
	}
	requireAsked(t, asked, 0)
}

// A company only another rep may open is never named to Ann's owner. The
// capture runs as the system, which sees it, so only the owner's sight stops it.
func TestACompanyTheOwnerCannotOpenAsksNothing(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	m := e.seedMovedContact(e.as(), t)
	e.execTx(e.as(), t, `UPDATE company SET owner_id = $2, visibility = 'owner' WHERE id = $1`, m.newco, e.otherRep)
	e.ensureMail(e.asNonHuman(principal.PrincipalSystem), t, "ann@newco.test", "newco.test", m.after(30))
	requireAsked(t, asked, 0)
}

// Mail only its participants may read, and the owner is not one, is not the
// owner's to be shown.
func TestMailTheOwnerMayNotReadAsksNothing(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	m := e.seedMovedContact(ctx, t)
	mail := e.sentMail(t, m, "inbound", true, party{"from", "ann@newco.test"})
	e.execTx(ctx, t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, mail.ActivityID)
	if _, err := e.store.EnsureCounterparty(ctx, mail); err != nil {
		t.Fatalf("ensuring the mail: %v", err)
	}
	requireAsked(t, asked, 0)
}

// One settled domain raises at most domainEmployerChangeCap questions, in one
// call to the inbox. The rest are asked when they next write.
func TestASettledDomainAsksABoundedNumberAtOnce(t *testing.T) {
	e := setupDedupe(t)
	asked := e.askingStore()
	ctx := e.as()
	oldco := e.createNamedCompany(ctx, t, "Oldco", "oldco.test")
	addresses := make([]string, 0, domainEmployerChangeCap+1)
	for i := range domainEmployerChangeCap + 1 {
		address := fmt.Sprintf("p%d@crowd.test", i)
		created, err := e.store.CreateContact(ctx, CreateContactInput{
			FullName: fmt.Sprintf("Person %d", i), Source: "manual",
			Emails: []ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
		})
		if err != nil {
			t.Fatal(err)
		}
		contact := ids.From[ids.ContactKind](ids.UUID(created.Id))
		if err := e.store.ResolveContactEmployer(ctx, contact, EmployerChoice{CompanyID: &oldco, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
		addresses = append(addresses, address)
	}
	var lastEmployed time.Time
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT max(created_at) FROM relationship WHERE kind = 'employment'`).Scan(&lastEmployed)
	}); err != nil {
		t.Fatal(err)
	}
	for _, address := range addresses {
		e.ensureMail(ctx, t, address, "crowd.test", lastEmployed.Add(time.Minute))
	}
	verdict, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "crowd.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: e.startTriageRead(ctx, t, "crowd.test"),
		DossierName: "Crowd GmbH", SeedURL: "https://crowd.test",
	})
	if err != nil || verdict.CompanyID == nil {
		t.Fatalf("resolve = %+v (err %v), want a company", verdict, err)
	}
	requireAsked(t, asked, domainEmployerChangeCap)
	if asked.calls != 1 {
		t.Fatalf("the inbox was called %d times for one domain, want once", asked.calls)
	}
}
