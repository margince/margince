// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEngagementOfReadsTheThreeStatesOffTheDirectionCounts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		inbound  int
		outbound int
		want     Engagement
	}{
		{"they wrote back", 2, 0, EngagementAnswered},
		{"they wrote back after we chased", 1, 4, EngagementAnswered},
		{"we wrote and heard nothing", 0, 3, EngagementNoReply},
		{"nobody has written at all", 0, 0, EngagementUntried},
		{"they wrote first and we never replied", 1, 0, EngagementAnswered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EngagementOf(RelationshipStrength{Inbound90d: tc.inbound, Outbound90d: tc.outbound})
			if got != tc.want {
				t.Fatalf("inbound=%d outbound=%d: got %q, want %q",
					tc.inbound, tc.outbound, got, tc.want)
			}
		})
	}
}

// An answered contact outranks an untried one and an untried one outranks a
// silent one — the order a rep triages in, and the reason untried sits in the
// middle rather than last: on an account where everyone has gone quiet, the
// person nobody has tried is the only move that is not another follow-up.
func TestRankContactsOrdersAnsweredThenUntriedThenNoReply(t *testing.T) {
	t.Parallel()
	silent := contactAt(t, "11111111-1111-4111-8111-111111111111", RelationshipStrength{Outbound90d: 5, Strength: 90})
	untried := contactAt(t, "22222222-2222-4222-8222-222222222222", RelationshipStrength{Strength: 10})
	answered := contactAt(t, "33333333-3333-4333-8333-333333333333", RelationshipStrength{Inbound90d: 1, Strength: 5})

	got := []ContactStrength{silent, untried, answered}
	RankContacts(got)

	want := []ids.PersonID{answered.PersonID, untried.PersonID, silent.PersonID}
	assertOrder(t, got, want)
}

// The silent contact here scores highest. If score led, it would sort first and
// the page would open on the one person whose next move needs a reason.
func TestRankContactsPutsEngagementAboveScore(t *testing.T) {
	t.Parallel()
	strongSilent := contactAt(t, "11111111-1111-4111-8111-111111111111", RelationshipStrength{Outbound90d: 9, Strength: 99})
	weakAnswered := contactAt(t, "22222222-2222-4222-8222-222222222222", RelationshipStrength{Inbound90d: 1, Strength: 1})

	got := []ContactStrength{strongSilent, weakAnswered}
	RankContacts(got)

	assertOrder(t, got, []ids.PersonID{weakAnswered.PersonID, strongSilent.PersonID})
}

func TestRankContactsBreaksAScoreTieByStrengthThenID(t *testing.T) {
	t.Parallel()
	weaker := contactAt(t, "11111111-1111-4111-8111-111111111111", RelationshipStrength{Inbound90d: 1, Strength: 40})
	strongerLateID := contactAt(t, "99999999-9999-4999-8999-999999999999", RelationshipStrength{Inbound90d: 1, Strength: 70})
	tiedEarlyID := contactAt(t, "22222222-2222-4222-8222-222222222222", RelationshipStrength{Inbound90d: 1, Strength: 70})

	got := []ContactStrength{weaker, strongerLateID, tiedEarlyID}
	RankContacts(got)

	// Both 70s come before the 40, and the tie between them resolves by id so
	// the order is the same on every page of a paged read.
	assertOrder(t, got, []ids.PersonID{tiedEarlyID.PersonID, strongerLateID.PersonID, weaker.PersonID})
}

// Paging over an unstable order shows a contact twice or never. Ranking the
// same set from two different starting arrangements must land identically.
func TestRankContactsIsStableAcrossInputOrder(t *testing.T) {
	t.Parallel()
	a := contactAt(t, "11111111-1111-4111-8111-111111111111", RelationshipStrength{Inbound90d: 1, Strength: 50})
	b := contactAt(t, "22222222-2222-4222-8222-222222222222", RelationshipStrength{Inbound90d: 1, Strength: 50})
	c := contactAt(t, "33333333-3333-4333-8333-333333333333", RelationshipStrength{Outbound90d: 1, Strength: 50})

	first := []ContactStrength{a, b, c}
	second := []ContactStrength{c, b, a}
	RankContacts(first)
	RankContacts(second)

	for i := range first {
		if first[i].PersonID != second[i].PersonID {
			t.Fatalf("position %d differs between input orders: %s vs %s",
				i, first[i].PersonID.UUID, second[i].PersonID.UUID)
		}
	}
}

func TestRankContactsHandlesAnEmptySet(t *testing.T) {
	t.Parallel()
	var none []ContactStrength
	RankContacts(none)
	if len(none) != 0 {
		t.Fatalf("ranking an empty set produced %d contacts", len(none))
	}
}

func contactAt(t *testing.T, id string, rs RelationshipStrength) ContactStrength {
	t.Helper()
	parsed, err := ids.Parse(id)
	if err != nil {
		t.Fatalf("parsing the fixture id %q: %v", id, err)
	}
	return ContactStrength{PersonID: ids.PersonID{UUID: parsed}, Strength: rs}
}

func assertOrder(t *testing.T, got []ContactStrength, want []ids.PersonID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d contacts, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].PersonID != want[i] {
			t.Fatalf("position %d: got %s, want %s", i, got[i].PersonID.UUID, want[i].UUID)
		}
	}
}
