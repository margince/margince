// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contact360_test

// The account card asks "This may be done" about the claim the contact page
// asks about. It keeps the answer in the contact page's store.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contactPageAsOf is the contact page service reading as of now.
func contactPageAsOf(e *integration.Env, now time.Time) *contact360.Service {
	return contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, consent.NewStore(e.DB()),
		comms.NewStore(e.DB(), func() time.Time { return now }, activities.NewStore(e.DB())), ai.NewFeedbackStore(e.DB()), func() time.Time { return now })
}

func contactMoment(as context.Context, t *testing.T, svc *contact360.Service, contactID ids.ContactID) crmcontracts.ContactMoment {
	t.Helper()
	page, err := svc.Assemble(as, contactID)
	if err != nil || page.Moment == nil {
		t.Fatalf("assembling the contact page: %v (moment %v)", err, page.Moment)
	}
	return *page.Moment
}

// promiseAtAccount is a promise thread whose contact works at an account, and
// our later reply on the thread, read a day after it.
func promiseAtAccount(t *testing.T) (*integration.Env, integration.PromiseThread, ids.CompanyID, ids.UUID, time.Time) {
	t.Helper()
	e := integration.Setup(t)
	p := e.SeedPromiseThread(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Kunde GmbH", &e.AdminUser))
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &p.Contact, CompanyID: &company, IsCurrentPrimary: new(true), Source: "manual"}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
	reply := e.LogThreadMail(e.Admin(), t, p.Contact, "thread-listing", p.Address, "outbound", p.SentAt.AddDate(0, 0, 6))
	return e, p, company, reply, p.SentAt.AddDate(0, 0, 7)
}

// accountPageAsOf is the company page service reading as of now, keeping its
// Not yet to Done? where the contact page keeps it, as the API wires it.
func accountPageAsOf(e *integration.Env, now time.Time) *company360.Service {
	return company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), func() time.Time { return now }).
		WithDoneAnswers(contactPageAsOf(e, now))
}

func accountMoment(ctx context.Context, t *testing.T, svc *company360.Service, company ids.CompanyID) crmcontracts.ContactMoment {
	t.Helper()
	page, err := svc.Assemble(ctx, company)
	if err != nil || page.Moment == nil {
		t.Fatalf("assembling the company page: %v (moment %v)", err, page.Moment)
	}
	return *page.Moment
}

func TestTheAccountCardAsksDoneAndNotYetBringsThePromiseBack(t *testing.T) {
	e, p, company, reply, now := promiseAtAccount(t)
	svc := accountPageAsOf(e, now)
	asked := accountMoment(e.Admin(), t, svc, company)
	if asked.MayBeDone == nil || ids.UUID(asked.MayBeDone.PromiseId) != ids.UUID(p.Claim.Id) || ids.UUID(asked.MayBeDone.EmailActivityId) != reply {
		t.Fatalf("the account card did not ask about the reply: %+v", asked)
	}
	if err := svc.DismissSuggestion(e.Admin(), company, asked.EvidenceFingerprint, nil); err != nil {
		t.Fatalf("answering Not yet: %v", err)
	}
	if plain := accountMoment(e.Admin(), t, svc, company); plain.MayBeDone != nil || plain.Rule != crmcontracts.ContactMomentRuleOpenPromise {
		t.Fatalf("Not yet did not bring the plain promise back: %+v", plain)
	}
}

// An agent reads the account card as its user left it, and what it puts away its
// user finds put away.
func TestAnAgentSharesItsUsersSetAsidesOnTheAccountCard(t *testing.T) {
	e, _, company, _, now := promiseAtAccount(t)
	svc := accountPageAsOf(e, now)
	agent := e.AgentFor(t, e.AdminUser, nil, integration.AdminPerms)
	asked := accountMoment(agent, t, svc, company)
	if asked.MayBeDone == nil {
		t.Fatalf("the agent's card did not ask about the reply: %+v", asked)
	}
	if err := svc.DismissSuggestion(e.Admin(), company, asked.EvidenceFingerprint, nil); err != nil {
		t.Fatalf("the user answering Not yet: %v", err)
	}
	plain := accountMoment(agent, t, svc, company)
	if plain.MayBeDone != nil || plain.Rule != crmcontracts.ContactMomentRuleOpenPromise {
		t.Fatalf("the agent's card ignored its user's Not yet: %+v", plain)
	}
	if err := svc.DismissSuggestion(agent, company, plain.EvidenceFingerprint, nil); err != nil {
		t.Fatalf("the agent setting the promise aside: %v", err)
	}
	for name, ctx := range map[string]context.Context{"the agent": agent, "its user": e.Admin()} {
		page, err := svc.Assemble(ctx, company)
		if err != nil {
			t.Fatalf("assembling the company page: %v", err)
		}
		if got := page.Moment; got != nil && got.Rule == crmcontracts.ContactMomentRuleOpenPromise {
			t.Errorf("%s still reads the promise the agent set aside: %+v", name, got)
		}
	}
}

// The account card and the contact page ask one question about one claim, so
// a Not yet on either page puts it away on both.
func TestNotYetToDoneOnOnePageHoldsOnTheOther(t *testing.T) {
	for _, answeredOn := range []string{"the account card", "the contact page"} {
		t.Run(answeredOn, func(t *testing.T) {
			e, p, company, _, now := promiseAtAccount(t)
			account, contactPage := accountPageAsOf(e, now), contactPageAsOf(e, now)
			asked, onContact := accountMoment(e.Admin(), t, account, company), contactMoment(e.Admin(), t, contactPage, p.Contact)
			if asked.MayBeDone == nil || asked.ClaimKey != onContact.ClaimKey || asked.EvidenceFingerprint != onContact.EvidenceFingerprint {
				t.Fatalf("the pages do not ask one question: account %+v, contact %+v", asked, onContact)
			}
			var err error
			if answeredOn == "the account card" {
				err = account.DismissSuggestion(e.Admin(), company, asked.EvidenceFingerprint, nil)
			} else {
				err = contactPage.DismissMoment(e.Admin(), p.Contact, crmcontracts.DismissContactMomentRequest{
					ClaimKey: onContact.ClaimKey, EvidenceFingerprint: onContact.EvidenceFingerprint,
				})
			}
			if err != nil {
				t.Fatalf("answering Not yet on %s: %v", answeredOn, err)
			}
			if got := accountMoment(e.Admin(), t, account, company); got.MayBeDone != nil {
				t.Errorf("the account card still asks after Not yet on %s: %+v", answeredOn, got.MayBeDone)
			}
			if got := contactMoment(e.Admin(), t, contactPage, p.Contact); got.MayBeDone != nil {
				t.Errorf("the contact page still asks after Not yet on %s: %+v", answeredOn, got.MayBeDone)
			}
		})
	}
}
