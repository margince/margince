// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package suggestionlanes

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheOwnerDialsNarrowTheSuggestionsToTheCompanyOwner(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	companies := seedSuggestions(t, e, 2, &e.Rep1, &e.Rep3)
	mine := companies[0]
	rep1, team1 := ids.From[ids.UserKind](e.Rep1), ids.From[ids.TeamKind](e.Team1)
	for name, query := range map[string]deals.SuggestionQuery{
		"owner_id":      {OwnerID: &rep1},
		"owner_team_id": {OwnerTeamID: &team1},
	} {
		got, _, err := e.Deals.ListSuggestions(e.Admin(), query)
		if err != nil {
			t.Fatalf("listing by %s: %v", name, err)
		}
		if len(got) != 1 || got[0].CompanyID != mine {
			t.Errorf("listing by %s = %v, want only the suggestion for %v", name, got, mine)
		}
	}
	unowned := true
	_, _, err := e.Deals.ListSuggestions(e.Admin(), deals.SuggestionQuery{OwnerID: &rep1, Unassigned: &unowned})
	if _, ok := errors.AsType[*storekit.PredicateError](err); !ok {
		t.Errorf("an owner and the unassigned queue at once = %v, want the conflicting filters refused", err)
	}
}

// The Worklist's owner scope narrows the count with the list, in the store and
// through the Worklist's binding to it. A scope never counts a suggestion its
// page could not show.
func TestOwnerScopeNarrowsTheSuggestionCountWithTheList(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	companies := seedSuggestions(t, e, 3, &e.Rep1, &e.Rep3)
	mine, theirs, nobodys := companies[0], companies[1], companies[2]
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](nobodys), contacts.UpdateCompanyInput{Clear: []string{"owner_id"}}); err != nil {
		t.Fatalf("clearing the unassigned account's owner: %v", err)
	}
	for name, tc := range map[string]struct {
		owners *storekit.OwnerSet
		want   []ids.UUID
	}{
		"one owner":   {&storekit.OwnerSet{IDs: []ids.UUID{e.Rep1}}, []ids.UUID{mine}},
		"unassigned":  {&storekit.OwnerSet{Unassigned: true}, []ids.UUID{nobodys}},
		"nobody":      {&storekit.OwnerSet{}, nil},
		"every owner": {nil, []ids.UUID{mine, theirs, nobodys}},
	} {
		query := deals.SuggestionQuery{ScopeOwners: tc.owners}
		got, _, err := e.Deals.ListSuggestions(e.Admin(), query)
		if err != nil {
			t.Fatalf("listing for %s: %v", name, err)
		}
		open, err := e.Deals.CountOpenSuggestions(e.Admin(), query)
		if err != nil {
			t.Fatalf("counting for %s: %v", name, err)
		}
		companies := map[ids.UUID]bool{}
		for _, suggestion := range got {
			companies[suggestion.CompanyID] = true
		}
		if open != len(got) || len(got) != len(tc.want) {
			t.Errorf("%s: listed %d and counted %d, want %d", name, len(got), open, len(tc.want))
		}
		seam := Deals{Store: e.Deals}
		page, complete, err := seam.OpenSuggestions(e.Admin(), tc.owners, 100)
		if err != nil || !complete {
			t.Fatalf("the Worklist listing for %s: complete %v, %v", name, complete, err)
		}
		if seamOpen, err := seam.CountOpen(e.Admin(), tc.owners); err != nil || seamOpen != len(tc.want) || len(page) != len(tc.want) {
			t.Errorf("%s: the Worklist listed %d and counted %d (err %v), want %d", name, len(page), seamOpen, err, len(tc.want))
		}
		if len(tc.want) > 1 {
			if short, complete, err := seam.OpenSuggestions(e.Admin(), tc.owners, len(tc.want)-1); err != nil || complete || len(short) != len(tc.want)-1 {
				t.Errorf("%s: a page one short of %d is %d rows, complete %v (%v); want incomplete", name, len(tc.want), len(short), complete, err)
			}
		}
		for _, company := range tc.want {
			if !companies[company] {
				t.Errorf("%s: the suggestion for %v is missing from %v", name, company, got)
			}
		}
	}
}
