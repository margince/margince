// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The mail hooks against what they must not reach. That is archived records,
// records the mailbox owner may not see, and a core table. A meeting that is
// no email gets its own case. Every record is made through its owning store.

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// partiesNaming is a parties hook that replaces both lists and names a reason.
func partiesNaming(contacts, companies []string, reason string) extension.MailPartiesHook {
	return func(_ context.Context, _ extension.MailView, p extension.MailParties) (extension.MailParties, error) {
		p.Contacts, p.Companies, p.Reason = contacts, companies, reason
		return p, nil
	}
}

// archiver may archive the records a deal world makes.
func archiver(w dealWorld) context.Context {
	grant := principal.ObjectGrant{Read: true, Update: true, Delete: true}
	return asColleague(w, principal.PrincipalHuman, map[string]principal.ObjectGrant{"company": grant, "contact": grant, "deal": grant})
}

// asSeat is the world as another seat of the workspace works in it.
func asSeat(w dealWorld, user ids.UUID) dealWorld {
	actor, _ := principal.Actor(w.ctx)
	actor.ID, actor.UserID = "human:"+user.String(), user
	w.ctx = principal.WithActor(w.ctx, actor)
	return w
}

// makePrivate narrows a company or contact to its owner through the store, as
// the owner's own edit does.
func makePrivate(t *testing.T, w dealWorld, company, contact ids.UUID) {
	t.Helper()
	owner := "owner"
	if company != (ids.UUID{}) {
		if _, err := w.contacts.UpdateCompany(w.ctx, ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{Visibility: &owner}); err != nil {
			t.Fatalf("making the company private: %v", err)
		}
	}
	if contact != (ids.UUID{}) {
		if _, err := w.contacts.UpdateContact(w.ctx, ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Visibility: &owner}); err != nil {
			t.Fatalf("making the contact private: %v", err)
		}
	}
}

// A parties hook cannot name an archived company or contact. The answer is
// ignored and the trace says so. The mail keeps the deal core chose.
func TestAPartiesHookCannotNameAnArchivedCompanyOrContact(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	acme := w.deal(t, "Acme rollout")
	gone := w.at(t, "Gone")
	leaver := w.employee(t, "carol@gone.example", gone.customer)
	if _, err := w.contacts.ArchiveContact(archiver(w), ids.From[ids.ContactKind](leaver), nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if _, err := w.contacts.ArchiveCompany(archiver(w), ids.From[ids.CompanyKind](gone.customer), nil); err != nil {
		t.Fatalf("archiving the company: %v", err)
	}
	for i, hook := range []extension.MailPartiesHook{
		partiesNaming(nil, []string{gone.customer.String()}, "other_account"),
		partiesNaming([]string{leaver.String()}, nil, "other_account"),
	} {
		registerMailHooks(t, extension.MailHooks{Parties: hook})
		id := []string{"arch1@acme.example", "arch2@acme.example"}[i]
		env.sync(t, mailFrom("alice@acme.example", id, "", "hello"))
		wantDeals(t, e, id, acme)
		wantFiling(t, e, activityOf(t, e, id), "ignored", "other_account")
	}
}

// A deal hook cannot name a deal that was archived: it is no longer open.
func TestADealHookCannotNameAnArchivedDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	retired := w.deal(t, "Acme retired")
	if _, err := w.store.ArchiveDeal(archiver(w), ids.From[ids.DealKind](retired), nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, _ extension.MailView, _ extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{Outcome: "linked", Deal: retired.String(), Reason: "stale"}, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "arch3@acme.example", "", "hello"))
	wantDeals(t, e, "arch3@acme.example", only)
	wantFiling(t, e, activityOf(t, e, "arch3@acme.example"), "ignored", "stale")
}

// A parties hook cannot name a contact the mailbox owner may not see.
func TestAPartiesHookCannotNameAContactTheMailboxOwnerCannotSee(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	acme := w.deal(t, "Acme rollout")
	theirs := asSeat(w, e.Rep3)
	hidden := theirs.employee(t, "dora@acme.example", w.customer)
	makePrivate(t, theirs, ids.UUID{}, hidden)
	registerMailHooks(t, extension.MailHooks{Parties: partiesNaming([]string{hidden.String()}, nil, "other_account")})
	env.sync(t, mailFrom("alice@acme.example", "priv1@acme.example", "", "hello"))
	wantDeals(t, e, "priv1@acme.example", acme)
	wantFiling(t, e, activityOf(t, e, "priv1@acme.example"), "ignored", "other_account")
}

// A meeting reaches the deal hook too, marked as one, and its answer counts.
func TestADealHookSeesAMeetingAsOneAndCanKeepItOffTheDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme rollout")
	var meetings atomic.Int32
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, m extension.MailView, _ extension.MailDeal) (extension.MailDeal, error) {
		if m.Meeting {
			meetings.Add(1)
		}
		return extension.MailDeal{Outcome: "none", Reason: "credit_hold"}, nil
	}})
	env.syncAsKind(t, map[string]string{"meet2@acme.example": "meeting"},
		mailFrom("alice@acme.example", "meet2@acme.example", "", "Planning"))
	if meetings.Load() == 0 {
		t.Fatal("the hook never saw the meeting as one")
	}
	wantDeals(t, e, "meet2@acme.example")
	wantFiling(t, e, activityOf(t, e, "meet2@acme.example"), "none", "credit_hold")
}

// A hook's Lookup runs as the unit role, which holds nothing on core tables
// such as system_log. The read is refused, and the unit's answer decides.
func TestAHooksLookupCannotReadSystemLog(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	var refusal atomic.Value
	registerMailHooks(t, extension.MailHooks{Deal: func(_ context.Context, m extension.MailView, d extension.MailDeal) (extension.MailDeal, error) {
		err := m.Lookup(`SELECT count(*) FROM system_log`).Scan(new(int))
		refusal.Store(errors.Is(err, extension.ErrCoreTableRefused))
		return d, nil
	}})
	env.sync(t, mailFrom("alice@acme.example", "sys1@acme.example", "", "hello"))
	if refused, _ := refusal.Load().(bool); !refused {
		t.Fatal("the lookup of system_log was not refused as a core table")
	}
	if !slices.Equal(dealsOf(t, e, "sys1@acme.example"), []ids.UUID{only}) {
		t.Fatal("the unchanged answer did not leave the mail on the open deal")
	}
}
