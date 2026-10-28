// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Creating a company with domain D offers the live contacts on D, and linking
// writes edges for the ticked ones only.
func TestANewCompanyOffersItsDomainContactsAndLinksOnlyTheTickedOnes(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	// The company carries a consumer domain too: an address there must never match.
	company := e.createNamedCompany(ctx, t, "Candco", "candco.test", "gmail.com")

	alice := e.contactWithEmail(ctx, t, "Alice Able", "alice@candco.test")
	bob := e.contactWithEmail(ctx, t, "Bob Baker", "bob@candco.test")
	gmail := e.contactWithEmail(ctx, t, "Carol Consumer", "carol.candco@gmail.com")
	dave := e.contactWithEmail(ctx, t, "Dave Already", "dave@candco.test")
	if _, err := e.store.LinkContactEmployer(ctx, dave, LinkEmployerInput{CompanyID: &company}); err != nil {
		t.Fatalf("linking dave ahead of time: %v", err)
	}
	erin := e.contactWithEmail(e.asOther(), t, "Erin Hidden", "erin@candco.test")
	if _, err := e.store.UpdateContact(e.asOther(), erin, UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("making erin private: %v", err)
	}

	candidates, truncated, err := e.store.ListLinkCandidates(ctx, company)
	if err != nil {
		t.Fatalf("listing candidates: %v", err)
	}
	if truncated {
		t.Error("truncated with two candidates")
	}
	if got := candidateIDs(candidates); len(got) != 2 || got[0] != alice || got[1] != bob {
		t.Fatalf("candidates = %v, want [alice bob] in name order (not %v the consumer address, %v already linked, %v unreadable)",
			got, gmail, dave, erin)
	}
	if candidates[0].Email != "alice@candco.test" {
		t.Errorf("matched email = %q, want alice@candco.test", candidates[0].Email)
	}

	edges, err := e.store.LinkCompanyContacts(ctx, company, []ids.UUID{alice.UUID})
	if err != nil {
		t.Fatalf("linking the ticked contact: %v", err)
	}
	if len(edges) != 1 || edges[0].ContactID == nil || *edges[0].ContactID != alice {
		t.Fatalf("edges = %+v, want one for alice", edges)
	}
	assertEmployedAt(ctx, t, e, alice, company)
	if got := e.employmentsOf(ctx, t, bob); got != 0 {
		t.Errorf("bob, who was not ticked, holds %d employment(s)", got)
	}

	left, _, err := e.store.ListLinkCandidates(ctx, company)
	if err != nil {
		t.Fatalf("listing candidates again: %v", err)
	}
	if got := candidateIDs(left); len(got) != 1 || got[0] != bob {
		t.Errorf("candidates after linking alice = %v, want [bob]", got)
	}
}

// One non-candidate refuses the whole batch by name, and nothing is written for
// the candidate beside it. An unreadable contact reads like any non-candidate.
func TestLinkingANonCandidateWritesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.createNamedCompany(ctx, t, "Batchco", "batchco.test")
	ready := e.contactWithEmail(ctx, t, "Rita Ready", "rita@batchco.test")
	elsewhere := e.contactWithEmail(ctx, t, "Olaf Elsewhere", "olaf@otherplace.test")
	hidden := e.contactWithEmail(e.asOther(), t, "Hana Hidden", "hana@batchco.test")
	if _, err := e.store.UpdateContact(e.asOther(), hidden, UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("making hana private: %v", err)
	}

	for name, offender := range map[string]ids.ContactID{"off the domain": elsewhere, "unreadable": hidden} {
		t.Run(name, func(t *testing.T) {
			_, err := e.store.LinkCompanyContacts(ctx, company, []ids.UUID{ready.UUID, offender.UUID})
			field, code, details := validationField(err)
			if field != fieldContactIDs || code != codeNotLinkCandidate || details["contact_id"] != offender.String() {
				t.Fatalf("err = %v, want 422 %s naming %s", err, codeNotLinkCandidate, offender)
			}
			if got := e.employmentsOf(ctx, t, ready); got != 0 {
				t.Errorf("the candidate beside the refused id holds %d employment(s), want 0", got)
			}
		})
	}
}

// The list matches a domain as the daily plant does: a subdomain address is a
// candidate, a lookalike ending is not.
func TestASubdomainAddressIsALinkCandidateOfTheParentDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.createNamedCompany(ctx, t, "Acme", "acme.example")
	regional := e.contactWithEmail(ctx, t, "Eva Europe", "eva@eu.acme.example")
	e.contactWithEmail(ctx, t, "Lou Lookalike", "lou@notacme.example")

	candidates, _, err := e.store.ListLinkCandidates(ctx, company)
	if err != nil {
		t.Fatalf("listing candidates: %v", err)
	}
	if got := candidateIDs(candidates); len(got) != 1 || got[0] != regional {
		t.Fatalf("candidates = %v, want only %v on eu.acme.example", got, regional)
	}
	if _, err := e.store.LinkCompanyContacts(ctx, company, []ids.UUID{regional.UUID}); err != nil {
		t.Fatalf("linking the subdomain candidate: %v", err)
	}
	assertEmployedAt(ctx, t, e, regional, company)
}

// A workspace seat on the company's domain is a colleague the plant never attaches.
// The list never offers them, and the link refuses them.
func TestAWorkspaceSeatIsNoLinkCandidate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.createNamedCompany(ctx, t, "Seatco", "seatco.test")
	seat := e.contactWithEmail(ctx, t, "Sam Seat", "sam@seatco.test")
	e.declareSeatAddress(ctx, t, e.otherRep, "sam@seatco.test", "provider")
	client := e.contactWithEmail(ctx, t, "Cleo Client", "cleo@seatco.test")

	candidates, _, err := e.store.ListLinkCandidates(ctx, company)
	if err != nil {
		t.Fatalf("listing candidates: %v", err)
	}
	if got := candidateIDs(candidates); len(got) != 1 || got[0] != client {
		t.Fatalf("candidates = %v, want only %v and not the seat %v", got, client, seat)
	}
	_, err = e.store.LinkCompanyContacts(ctx, company, []ids.UUID{seat.UUID})
	if field, code, _ := validationField(err); field != fieldContactIDs || code != codeNotLinkCandidate {
		t.Fatalf("linking the seat = %v, want 422 %s", err, codeNotLinkCandidate)
	}
}

// A contact shared read-only can be read but not given an edge. It is not offered,
// and the link refuses it like any other non-candidate.
func TestAReadOnlyShareIsNoLinkCandidate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.asRepOwnRows()
	company := e.createNamedCompany(ctx, t, "Shareco", "shareco.test")
	mine := e.contactWithEmail(ctx, t, "Mia Mine", "mia@shareco.test")
	readOnly := e.contactWithEmail(e.asOther(), t, "Rob Readonly", "rob@shareco.test")
	writable := e.contactWithEmail(e.asOther(), t, "Wes Writable", "wes@shareco.test")
	e.contactWithEmail(e.asOther(), t, "Una Unshared", "una@shareco.test")
	e.grantContact(ctx, t, readOnly, "read")
	e.grantContact(ctx, t, writable, "write")

	candidates, _, err := e.store.ListLinkCandidates(ctx, company)
	if err != nil {
		t.Fatalf("listing candidates: %v", err)
	}
	if got := candidateIDs(candidates); len(got) != 2 || got[0] != mine || got[1] != writable {
		t.Fatalf("candidates = %v, want [%v %v] and not the read-only share %v", got, mine, writable, readOnly)
	}
	_, err = e.store.LinkCompanyContacts(ctx, company, []ids.UUID{mine.UUID, readOnly.UUID})
	if field, code, details := validationField(err); field != fieldContactIDs || code != codeNotLinkCandidate ||
		details["contact_id"] != readOnly.String() {
		t.Fatalf("linking the read-only share = %v, want 422 %s naming it", err, codeNotLinkCandidate)
	}
	if got := e.employmentsOf(ctx, t, mine); got != 0 {
		t.Errorf("the candidate beside the refused share holds %d employment(s), want 0", got)
	}
}

// A caller the link would refuse outright is offered no one.
func TestACallerWhoMayNotLinkIsOfferedNoCandidates(t *testing.T) {
	e := setupDedupe(t)
	company := e.createNamedCompany(e.as(), t, "Viewco", "viewco.test")
	e.contactWithEmail(e.as(), t, "Vic Viewed", "vic@viewco.test")
	actor, _ := principal.Actor(e.as())
	actor.Permissions.Objects["relationship"] = principal.ObjectGrant{Read: true}
	ctx := principal.WithActor(e.as(), actor)

	candidates, truncated, err := e.store.ListLinkCandidates(ctx, company)
	if err != nil || len(candidates) != 0 || truncated {
		t.Fatalf("candidates = %v truncated = %v err = %v, want none for a caller without relationship:create", candidateIDs(candidates), truncated, err)
	}
}

// asRepOwnRows is the rep of as() seeing only their own rows and what is shared with them.
func (e *dedupeEnv) asRepOwnRows() context.Context {
	ctx := e.as()
	actor, _ := principal.Actor(ctx)
	actor.Permissions.RowScope = principal.RowScopeOwn
	return principal.WithActor(ctx, actor)
}

// grantContact shares a colleague's contact with the rep at access.
func (e *dedupeEnv) grantContact(ctx context.Context, t *testing.T, contactID ids.ContactID, access string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
			VALUES ('contact', $1, 'user', $2, $3, $4)`, contactID, e.rep, access, e.otherRep)
		return err
	}); err != nil {
		t.Fatalf("sharing a contact at %s: %v", access, err)
	}
}

func candidateIDs(candidates []LinkCandidate) []ids.ContactID {
	out := make([]ids.ContactID, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.ContactID)
	}
	return out
}
