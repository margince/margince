// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Answered is earned by replying, not by receiving: a contact whose latest
// message has no outbound after it reads as waiting however much traffic the
// window holds, because one unprompted mail nobody answered is not a success.
func TestEngagementOfReadsTheFourStatesOffWhoWroteLast(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	at := func(daysLater int) *time.Time {
		v := base.AddDate(0, 0, daysLater)
		return &v
	}
	for _, tc := range []struct {
		name string
		rs   RelationshipStrength
		want Engagement
	}{
		{
			"we replied to their latest mail",
			RelationshipStrength{Inbound90d: 1, Outbound90d: 1, LastInbound: at(0), LastOutbound: at(1)},
			EngagementAnswered,
		},
		{
			"they wrote again after our reply",
			RelationshipStrength{Inbound90d: 2, Outbound90d: 1, LastInbound: at(2), LastOutbound: at(1)},
			EngagementWaiting,
		},
		{
			"they wrote first and we never replied",
			RelationshipStrength{Inbound90d: 1, LastInbound: at(0)},
			EngagementWaiting,
		},
		{
			"their mail and ours carry one timestamp",
			RelationshipStrength{Inbound90d: 1, Outbound90d: 1, LastInbound: at(0), LastOutbound: at(0)},
			EngagementWaiting,
		},
		{
			"we wrote and heard nothing",
			RelationshipStrength{Outbound90d: 3, LastOutbound: at(0)},
			EngagementNoReply,
		},
		{
			"their reply aged out of the window and we chased since",
			RelationshipStrength{Outbound90d: 1, LastInbound: at(-200), LastOutbound: at(0)},
			EngagementNoReply,
		},
		{
			"nobody has written at all",
			RelationshipStrength{},
			EngagementUntried,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := EngagementOf(tc.rs); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A contact waiting on our reply outranks one we answered, an answered one
// outranks an untried one, and untried outranks silent — the order a rep
// triages in. Untried sits above no-reply because on an account where everyone
// has gone quiet, the person nobody has tried is the only move that is not
// another follow-up.
func TestRankContactsOrdersWaitingThenAnsweredThenUntriedThenNoReply(t *testing.T) {
	t.Parallel()
	inAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	outAt := inAt.Add(time.Hour)
	silent := contactAt(t, "11111111-1111-4111-8111-111111111111", RelationshipStrength{Outbound90d: 5, LastOutbound: &outAt, Strength: 90})
	untried := contactAt(t, "22222222-2222-4222-8222-222222222222", RelationshipStrength{Strength: 10})
	answered := contactAt(t, "33333333-3333-4333-8333-333333333333", RelationshipStrength{Inbound90d: 1, Outbound90d: 1, LastInbound: &inAt, LastOutbound: &outAt, Strength: 5})
	waiting := contactAt(t, "44444444-4444-4444-8444-444444444444", RelationshipStrength{Inbound90d: 1, LastInbound: &inAt, Strength: 1})

	got := []ContactStrength{silent, untried, answered, waiting}
	RankContacts(got)

	want := []ids.PersonID{waiting.PersonID, answered.PersonID, untried.PersonID, silent.PersonID}
	assertOrder(t, got, want)
}

// The silent contact here scores highest. If score led, it would sort first and
// the page would open on the one person whose next move needs a reason.
func TestRankContactsPutsEngagementAboveScore(t *testing.T) {
	t.Parallel()
	inAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	outAt := inAt.Add(time.Hour)
	strongSilent := contactAt(t, "11111111-1111-4111-8111-111111111111", RelationshipStrength{Outbound90d: 9, LastOutbound: &outAt, Strength: 99})
	weakAnswered := contactAt(t, "22222222-2222-4222-8222-222222222222", RelationshipStrength{Inbound90d: 1, Outbound90d: 1, LastInbound: &inAt, LastOutbound: &outAt, Strength: 1})

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
