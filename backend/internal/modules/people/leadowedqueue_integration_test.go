// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The owed-a-reply dial belongs in the QUERY, and this is why.
//
// A caller reading this queue takes a bounded page. Answered leads sort by the
// same keys as unanswered ones when no first-response target is set, so a
// caller filtering after the cut loses every unanswered lead that sat behind an
// answered one — and reports the shortfall as "none owed" rather than as a page
// that was cut.
//
// TWO GUARDS hold this and the test proves the outcome rather than either one:
// the dial puts `first_response_at IS NULL` in the WHERE clause, and the band
// in leadQueueRank sorts answered leads last even with no target set. Removing
// one alone leaves this green; removing both returns an answered lead here.
// Both exist deliberately — the dial is what a caller asks for, the band is
// what every other reader of this queue gets without asking.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheOwedDialKeepsAnsweredLeadsOutOfTheBoundedPage(t *testing.T) {
	e := setupPromoteConsent(t)

	// Answered leads first, so an unfiltered read of one row returns one of
	// them and the unanswered lead is the one that would be lost.
	var answered []ids.UUID
	for _, name := range []string{"Answered A", "Answered B"} {
		lead, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{
			FullName: strPtr(name), Source: "manual",
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		answered = append(answered, ids.UUID(lead.Id))
	}
	replied := time.Now().UTC()
	for _, id := range answered {
		if _, err := e.owner.Exec(e.ctx,
			`UPDATE lead SET first_response_at = $2 WHERE id = $1`, id, replied); err != nil {
			t.Fatalf("stamping a first response: %v", err)
		}
	}
	owedLead, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{
		FullName: strPtr("Nobody has answered this one"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	// A page smaller than the answered set: without the dial this returns only
	// answered leads and the caller concludes nothing is owed.
	one := 1
	owed := true
	rows, _, err := e.store.ListLeads(e.ctx, ListLeadsInput{Limit: &one, OwedAReply: &owed})
	if err != nil {
		t.Fatalf("listing owed leads: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("owed page = %d rows, want the one unanswered lead", len(rows))
	}
	if ids.UUID(rows[0].Id) != ids.UUID(owedLead.Id) {
		t.Errorf("the bounded page returned %q, want the unanswered lead — an answered one "+
			"filled the cut", *rows[0].FullName)
	}
}
