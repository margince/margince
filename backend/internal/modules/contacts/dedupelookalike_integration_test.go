// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type companyAgreement struct{ held, asked NewCompany }

type contactAgreement struct {
	held     NewContact
	employer string
	asked    NewContact
}

// The batch half answers about a record not written yet what the ladder answers
// once it is. The cases cross the name tables of this package with the facts a
// pair can share. Each writes the held record through the store, asks both, and
// retires it, so the next case meets an estate without it.
func TestTheLadderAndItsBatchHalfAgree(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	inTx := func(fn func(pgx.Tx) error) error { return e.store.tx(ctx, fn) }
	for i, tc := range companyAgreements() {
		domain := fmt.Sprintf("d%d.example", i)
		tc.held.Domains = []string{domain}
		if i%7 == 0 {
			tc.asked.DisplayName = domain
		}
		in := CreateCompanyInput{
			DisplayName: tc.held.DisplayName, Source: "manual", Address: tc.held.Address,
			Domains: []CompanyDomainInput{{Domain: domain, IsPrimary: true}},
		}
		if tc.held.LegalName != "" {
			in.LegalName = &tc.held.LegalName
		}
		created, err := e.store.CreateCompany(ctx, in)
		if err != nil {
			t.Fatalf("create %+v: %v", tc.held, err)
		}
		ladder := e.dedupeCompanyInTx(ctx, t, CompanyCandidate{
			DisplayName: tc.asked.DisplayName, LegalName: tc.asked.LegalName, Address: tc.asked.Address,
		}).Decision != DecisionNoMatch
		if batch, err := CompanyLookAlike(ctx, inTx, tc.asked, tc.held); err != nil || batch != ladder {
			t.Errorf("%+v against %+v: batch half = %v (%v), ladder = %v", tc.asked, tc.held, batch, err, ladder)
		}
		e.retire(ctx, t, "company", ids.UUID(created.Id))
	}
	for _, tc := range contactAgreements() {
		in := CreateContactInput{FullName: tc.held.FullName, Source: "manual"}
		for _, email := range tc.held.Emails {
			in.Emails = append(in.Emails, ContactEmailInput{Email: email, EmailType: "work", IsPrimary: true})
		}
		if tc.employer != "" {
			employer, err := e.store.CreateCompany(ctx, CreateCompanyInput{
				DisplayName: "Employer " + tc.employer, Source: "manual",
				Domains: []CompanyDomainInput{{Domain: tc.employer, IsPrimary: true}},
			})
			if err != nil {
				t.Fatalf("create the employer: %v", err)
			}
			in.Employer = EmployerChoice{CompanyID: new(ids.From[ids.CompanyKind](ids.UUID(employer.Id))), Source: "manual"}
			tc.held.EmployerDomains = []string{tc.employer}
		}
		created, err := e.store.CreateContact(ctx, in)
		if err != nil {
			t.Fatalf("create %s: %v", tc.held.FullName, err)
		}
		ladder := e.dedupeInTx(ctx, t, ContactCandidate{FullName: tc.asked.FullName, Emails: tc.asked.Emails}).Decision != DecisionNoMatch
		if batch, err := ContactLookAlike(ctx, inTx, tc.asked, nil, tc.held); err != nil || batch != ladder {
			t.Errorf("%+v against %+v: batch half = %v (%v), ladder = %v", tc.asked, tc.held, batch, err, ladder)
		}
		e.retire(ctx, t, "contact", ids.UUID(created.Id))
	}
}

// companyAgreements cross every company name pair with one street, two
// streets, and the held company's registered name.
func companyAgreements() []companyAgreement {
	street := func(line string) *crmcontracts.Address {
		return &crmcontracts.Address{Line1: &line, PostalCode: new("80331"), City: new("München")}
	}
	var cases []companyAgreement
	for _, pair := range append(append([][2]string{}, duplicateCompanyNames...), unrelatedCompanyNames...) {
		held, asked := NewCompany{DisplayName: pair[0]}, NewCompany{DisplayName: pair[1]}
		atOne, atTwo := held, asked
		atOne.Address, atTwo.Address = street("Marienplatz 8"), street("marienplatz 8")
		apart := atTwo
		apart.Address = street("Sendlinger Straße 1")
		registered := NewCompany{DisplayName: "Zyxwvu Holding", LegalName: pair[0]}
		cases = append(cases, companyAgreement{held, asked}, companyAgreement{atOne, atTwo},
			companyAgreement{atOne, apart}, companyAgreement{registered, asked})
	}
	return cases
}

// contactAgreements cross every contact name pair with one employer's mail
// domain, a mail provider and the held contact's employer. One address then
// comes in two cases, and in two spellings of its domain.
func contactAgreements() []contactAgreement {
	var cases []contactAgreement
	for i, names := range contactNameCorroborationCases {
		work, employer, mailbox := fmt.Sprintf("w%d.example", i), fmt.Sprintf("e%d.example", i), fmt.Sprintf("%d@gmail.com", i)
		held, asked := NewContact{FullName: names.incumbent}, NewContact{FullName: names.candidate}
		for _, variant := range []struct{ held, asked, employer string }{
			{"held@" + work, "asked@" + work, ""}, {"held" + mailbox, "asked" + mailbox, ""}, {"kept" + mailbox, "asked@" + employer, employer},
		} {
			held.Emails, asked.Emails = []string{variant.held}, []string{variant.asked}
			cases = append(cases, contactAgreement{held, variant.employer, asked})
		}
		cases = append(cases, contactAgreement{
			NewContact{FullName: names.incumbent, Emails: []string{"same@" + work}},
			"",
			NewContact{FullName: "Bo Lind", Emails: []string{"SAME@" + work}},
		})
	}
	// Two typed letters swapped score over the bar and share few trigrams, so
	// only the candidate query's admission test keeps the ladder from scoring them.
	cases = append(cases, contactAgreement{
		NewContact{FullName: "John Doe", Emails: []string{"john@typo.example"}},
		"",
		NewContact{FullName: "Jhon Deo", Emails: []string{"jhon@typo.example"}},
	})
	return append(cases, contactAgreement{
		NewContact{FullName: "Anna Berg", Emails: []string{"anna@münchen.example"}},
		"",
		NewContact{FullName: "Bo Lind", Emails: []string{"anna@xn--mnchen-3ya.example"}},
	})
}

// retire archives a record the agreement test is done with. The ladder reads
// live rows only, and the domains and addresses of each case are its own.
func (e *dedupeEnv) retire(ctx context.Context, t *testing.T, table string, id ids.UUID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE `+pgx.Identifier{table}.Sanitize()+` SET archived_at = now() WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("retiring %s %s: %v", table, id, err)
	}
}

// A company the caller may not see lends no domain to a contact the batch
// files. That holds for the employer a row names and for the employer of a
// contact the row updates.
func TestAHiddenEmployerLendsTheBatchNoDomain(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	owner := e.as()
	hidden, err := e.store.CreateCompany(owner, CreateCompanyInput{
		DisplayName: "Hidden Works", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "hidden.example", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	hiddenID := ids.From[ids.CompanyKind](ids.UUID(hidden.Id))
	employed, err := e.store.CreateContact(owner, CreateContactInput{
		FullName: "Ada Held", Source: "manual",
		Employer: EmployerChoice{CompanyID: &hiddenID, Source: "manual"},
	})
	if err != nil {
		t.Fatal(err)
	}
	setRowVisibility(owner, t, e, `UPDATE company SET owner_id = $1, visibility = 'owner' WHERE id = $2`, &e.rep, hiddenID.UUID)
	colleague := e.asOther()
	preview, err := e.store.PreviewContactEmployer(colleague, CreateContactInput{
		FullName: "Bo Lind", Source: "manual",
		Employer: EmployerChoice{
			CompanyName: new("Hidden Works"), Source: "manual",
			Domains: []CompanyDomainInput{{Domain: "hidden.example", IsPrimary: true}},
		},
		Emails: []ContactEmailInput{{Email: "bo@hidden.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.CompanyID == nil || len(preview.EmployerDomains) != 0 {
		t.Errorf("the preview named %v with domains %v, want the hidden employer with none", preview.CompanyID, preview.EmployerDomains)
	}
	var held NewContact
	if err := e.store.tx(colleague, func(tx pgx.Tx) (err error) {
		held, err = ContactAfterUpdate(colleague, tx, ids.From[ids.ContactKind](ids.UUID(employed.Id)), []byte(`{"full_name":"Ada Held"}`), UpdateContactInput{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(held.EmployerDomains) != 0 {
		t.Errorf("an updated contact's hidden employer lent the batch its domains %v", held.EmployerDomains)
	}
}
