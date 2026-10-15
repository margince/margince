// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The order is the answer: a reader takes the first name as whom to ask. Two
// reads of an unchanged network have to agree, so the tiebreak is part of the
// promise and not an implementation detail.
func TestTheColleagueToAskComesFirstAndTiesBreakByName(t *testing.T) {
	t.Parallel()
	colleagues := []CompanyReachColleague{
		{DisplayName: "Zoe Adeyemi", Connections: 2},
		{DisplayName: "Ana Brandt", Connections: 9},
		{DisplayName: "Bo Castellanos", Connections: 2},
	}
	sortByReachThenName(colleagues)

	want := []string{"Ana Brandt", "Bo Castellanos", "Zoe Adeyemi"}
	for i, name := range want {
		if colleagues[i].DisplayName != name {
			t.Errorf("position %d is %q, want %q", i, colleagues[i].DisplayName, name)
		}
	}
}

// A tie is the case that decides whether the order is stable. Equal counts in
// the order a map handed them over would reorder between two identical reads.
func TestTwoReadsOfOneTieAgree(t *testing.T) {
	t.Parallel()
	first := []CompanyReachColleague{
		{DisplayName: "Bo Castellanos", Connections: 4},
		{DisplayName: "Ana Brandt", Connections: 4},
	}
	second := []CompanyReachColleague{
		{DisplayName: "Ana Brandt", Connections: 4},
		{DisplayName: "Bo Castellanos", Connections: 4},
	}
	sortByReachThenName(first)
	sortByReachThenName(second)

	for i := range first {
		if first[i].DisplayName != second[i].DisplayName {
			t.Fatalf("position %d differs: %q and %q",
				i, first[i].DisplayName, second[i].DisplayName)
		}
	}
	if first[0].DisplayName != "Ana Brandt" {
		t.Errorf("a tie leads with %q, want the name that sorts first", first[0].DisplayName)
	}
}

// An account nobody reaches has to serialise as an empty list. A null reads as
// a field that was never sent, which is a different claim from "nobody".
func TestAnAccountNobodyReachesAnswersAnEmptyList(t *testing.T) {
	t.Parallel()
	wire := companyReachWire(nil)
	if wire.Colleagues == nil {
		t.Fatal("colleagues is nil, want an empty list")
	}
	if len(wire.Colleagues) != 0 {
		t.Errorf("colleagues = %v, want empty", wire.Colleagues)
	}
}

// The count travels with the name. A colleague named against the wrong number
// sends a rep to ask the wrong colleague, which the screen cannot show as odd.
func TestEachColleagueKeepsTheirOwnCount(t *testing.T) {
	t.Parallel()
	ana, bo := ids.NewV7(), ids.NewV7()
	wire := companyReachWire([]CompanyReachColleague{
		{UserID: ana, DisplayName: "Ana Brandt", Connections: 9},
		{UserID: bo, DisplayName: "Bo Castellanos", Connections: 2},
	})

	if len(wire.Colleagues) != 2 {
		t.Fatalf("got %d colleagues, want 2", len(wire.Colleagues))
	}
	for _, want := range []struct {
		name  string
		count int
	}{{"Ana Brandt", 9}, {"Bo Castellanos", 2}} {
		var found bool
		for _, c := range wire.Colleagues {
			if c.DisplayName == want.name {
				found = true
				if c.Connections != want.count {
					t.Errorf("%s carries %d, want %d", want.name, c.Connections, want.count)
				}
			}
		}
		if !found {
			t.Errorf("%s is missing from the answer", want.name)
		}
	}
	if ids.UUID(wire.Colleagues[0].UserId) != ana {
		t.Errorf("the first colleague's id is not Ana's")
	}
}

// A visibility-only save has to leave a readable trace. The images are what
// the audit row carries. A field missing from them writes a row whose before
// and after read alike, and the record says nothing happened.
func TestTheAuditImagesCarryTheReachAnswer(t *testing.T) {
	t.Parallel()
	withheld := auditLinkedInAccount(LinkedInAccount{ReachVisibleToTeam: false})
	shared := auditLinkedInAccount(LinkedInAccount{ReachVisibleToTeam: true})

	answer, ok := withheld["reach_visible_to_team"]
	if !ok {
		t.Fatal("the images omit the reach answer, so withholding leaves no trace")
	}
	if answer != false {
		t.Errorf("withheld image says %v, want false", answer)
	}
	if shared["reach_visible_to_team"] != true {
		t.Errorf("shared image says %v, want true", shared["reach_visible_to_team"])
	}
}
