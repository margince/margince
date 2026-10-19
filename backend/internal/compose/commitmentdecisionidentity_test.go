// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLegacyPromiseIdentitySurvivesUnresolvedAttribution(t *testing.T) {
	source, seat := ids.NewV7(), ids.NewV7()
	for _, party := range []string{"us", "name:", unnamedPromiserKey("")} {
		t.Run(party, func(t *testing.T) {
			c := Commitment{SourceActivityID: source, Span: "3:I will send pricing.", Party: "someone"}
			offer := CommitmentTaskProposal{SourceActivityID: source, Locator: commitmentLocator(source, claimKindOurs, party, c.Span)}
			if !samePromiseBeforeAttributionChanged(c, offer) {
				t.Fatal("legacy decision was lost")
			}
			c.Seat = &seat
			if !samePromiseBeforeAttributionChanged(c, offer) {
				t.Fatal("resolved attribution lost the legacy decision")
			}
			c.Span = "4:I will send pricing."
			if samePromiseBeforeAttributionChanged(c, offer) {
				t.Fatal("a separate clause inherited the decision")
			}
			c.Span = "3:I will send pricing."
			c.SourceActivityID = ids.NewV7()
			if samePromiseBeforeAttributionChanged(c, offer) {
				t.Fatal("another source inherited the decision")
			}
		})
	}
}

func TestReportedNamesNeedAnUnambiguousVerifiedParticipant(t *testing.T) {
	seat, other := ids.NewV7(), ids.NewV7()
	c := Commitment{Seat: &seat, Party: "priya@example.test", parties: []commitmentParty{
		{Ref: "seat:" + seat.String(), Name: "Priya Raman", Address: "priya@example.test"},
		{Ref: "seat:" + other.String(), Name: "Mira Raman", Address: "mira@example.test"},
	}}
	for _, name := range []string{"Priya", "Priya Raman", "priya@example.test"} {
		if !sameReportedPromiser(name, c) {
			t.Fatalf("verified participant lost for %s", name)
		}
	}
	for _, name := range []string{"Raman", "Mira", "Mira Raman"} {
		if sameReportedPromiser(name, c) {
			t.Fatalf("ambiguous or different participant matched: %s", name)
		}
	}
	c.parties[1].Name = "Priya Shah"
	if sameReportedPromiser("Priya", c) {
		t.Fatal("shared first name matched a specific participant")
	}
}

func TestLegacyProposalAssigneeDoesNotReplaceItsPromiserIdentity(t *testing.T) {
	source, promiser, reviewer := ids.NewV7(), ids.NewV7(), ids.NewV7()
	c := Commitment{SourceActivityID: source, Span: "3:I will send pricing.", parties: []commitmentParty{
		{Ref: "seat:" + promiser.String(), Seat: &promiser, Role: "attendee"},
	}}
	for _, assignee := range []*ids.UUID{nil, &reviewer} {
		offer := CommitmentTaskProposal{
			SourceActivityID: source, SeatID: assignee,
			Locator: commitmentLocator(source, claimKindOurs, "seat:"+promiser.String(), c.Span),
		}
		if !samePromiseBeforeAttributionChanged(c, offer) {
			t.Fatal("legacy reviewer assignment replaced the verified promiser")
		}
	}
}
