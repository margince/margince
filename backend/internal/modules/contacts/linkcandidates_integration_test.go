// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

func candidateIDs(candidates []LinkCandidate) []ids.ContactID {
	out := make([]ids.ContactID, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.ContactID)
	}
	return out
}
