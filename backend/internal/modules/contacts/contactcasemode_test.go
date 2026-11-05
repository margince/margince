// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// caseOf builds a case whose members carry the given owners, joined in a
// chain of open pairs. A nil owner is unowned; live says the owner is a live human.
func caseOf(owners []*ids.UUID, live []bool) contactCase {
	var c contactCase
	for i, owner := range owners {
		c.Members = append(c.Members, caseMember{ID: ids.NewV7(), OwnerID: owner, OwnerLive: live[i]})
		if i > 0 {
			c.Edges = append(c.Edges, caseEdge{Left: c.Members[i-1].ID, Right: c.Members[i].ID, Disposition: dispositionOpen})
		}
	}
	return c
}

func readerOf(c contactCase, writable ...int) caseAuthority {
	a := caseAuthority{c: c, mayAct: true, writable: map[ids.UUID]bool{}}
	for _, i := range writable {
		a.writable[c.Members[i].ID] = true
	}
	return a
}

// The case route names an owner for the result. A pair whose survivor can have
// no live owner offers no case merge. The pair route may still merge it.
func TestACasePairNeedsALiveOwnerForWhatSurvives(t *testing.T) {
	t.Parallel()
	mine, theirs := ids.NewV7(), ids.NewV7()
	cases := []struct {
		name     string
		c        contactCase
		writable []int
		want     crmcontracts.ContactCaseMergeMode
	}{
		{
			name:     "the colleague's record has a live owner",
			c:        caseOf([]*ids.UUID{&mine, &theirs}, []bool{true, true}),
			writable: []int{0},
			want:     crmcontracts.ContactCaseMergeModePair,
		},
		{
			name:     "the record that must survive is unowned",
			c:        caseOf([]*ids.UUID{&mine, nil}, []bool{true, false}),
			writable: []int{0},
			want:     crmcontracts.ContactCaseMergeModeNone,
		},
		{
			name:     "the record that must survive has no live owner",
			c:        caseOf([]*ids.UUID{&mine, &theirs}, []bool{true, false}),
			writable: []int{0},
			want:     crmcontracts.ContactCaseMergeModeNone,
		},
		{
			name:     "either may survive and neither has a live owner",
			c:        caseOf([]*ids.UUID{&theirs, &mine, &mine}, []bool{false, false, false}),
			writable: []int{1, 2},
			want:     crmcontracts.ContactCaseMergeModeNone,
		},
		{
			name:     "either may survive and one has a live owner",
			c:        caseOf([]*ids.UUID{&theirs, &mine, &mine}, []bool{false, false, true}),
			writable: []int{1, 2},
			want:     crmcontracts.ContactCaseMergeModePair,
		},
	}
	for _, tc := range cases {
		a := readerOf(tc.c, tc.writable...)
		if got := a.mergeMode(); got != tc.want {
			t.Errorf("%s: merge_mode = %s, want %s", tc.name, got, tc.want)
		}
		if !a.mergeOffer(tc.c.Edges[len(tc.c.Edges)-1]).May {
			t.Errorf("%s: the pair route no longer offers the merge", tc.name)
		}
	}
}
