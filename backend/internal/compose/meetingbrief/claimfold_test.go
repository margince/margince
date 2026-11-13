// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

const sessionPromise = "Lan Pham scheduled an introductory session on the archive solution for Northwind on October 30, 2026."

func promiseFor(body string, due time.Time) crmcontracts.ConversationClaim {
	return crmcontracts.ConversationClaim{
		Kind: crmcontracts.ConversationClaimKind(kindCommitmentOurs), Body: body,
		Status: crmcontracts.ConversationClaimStatus(statusOpen), DueAt: &due,
		SourceActivityId: openapi_types.UUID(ids.NewV7()),
	}
}

// One invitation sent to three attendees is filed as a promise to each of
// them. The second mailbox that received it was read in other words. The brief
// asks about it once and names all three.
func TestOnePromiseToSeveralAttendeesIsAskedOnce(t *testing.T) {
	due := at(20)
	names := []string{"Ana Roth", "Ben Ito", "Cleo Marx"}
	room := meeting{ID: ids.MustParse(meetingID), Subject: "Second demo", StartsAt: at(12)}
	perAttendee := map[ids.UUID][]crmcontracts.ConversationClaim{}
	for _, name := range names {
		contact := ids.NewV7()
		room.Attendees = append(room.Attendees, attendeeRow{ContactID: contact, FullName: name})
		perAttendee[contact] = []crmcontracts.ConversationClaim{promiseFor(sessionPromise, due)}
	}
	reworded := "Lan Pham will conduct an introduction and clarification session on the archive solution on October 30, 2026."
	perAttendee[room.Attendees[1].ContactID] = append(perAttendee[room.Attendees[1].ContactID],
		promiseFor(reworded, due),
		promiseFor("Lan Pham will send the pricing sheet.", due))

	in := FromMeeting(room, perAttendee, at(10))
	in.Zone = time.UTC
	asks := likelyAsksFor(in, rankClaims(in))
	if len(asks) != 2 {
		t.Fatalf("want the session and the pricing sheet once each, got %d: %+v", len(asks), asks)
	}
	session := asks[0]
	if !strings.Contains(session.Question, "archive solution") {
		session = asks[1]
	}
	if want := `Where does this stand: "` + strings.TrimSuffix(sessionPromise, ".") + `"?`; session.Question != want {
		t.Errorf("question = %q, want %q", session.Question, want)
	}
	if want := "We promised Ana Roth, Ben Ito and Cleo Marx this and it is still open."; session.Basis.Text != want {
		t.Errorf("basis = %q, want %q", session.Basis.Text, want)
	}
}

func TestClaimsThatOnlyReadAlikeStayApart(t *testing.T) {
	due, later := at(20), at(21)
	base := ClaimIn{ContactName: "Ana Roth", Kind: kindCommitmentOurs, Status: statusOpen, Body: sessionPromise, DueAt: &due}
	for name, other := range map[string]ClaimIn{
		"another day":   {ContactName: "Ben Ito", Kind: kindCommitmentOurs, Status: statusOpen, Body: strings.Replace(sessionPromise, "introductory", "follow-up", 1), DueAt: &later},
		"no due date":   {ContactName: "Ben Ito", Kind: kindCommitmentOurs, Status: statusOpen, Body: strings.Replace(sessionPromise, "introductory", "follow-up", 1)},
		"already kept":  {ContactName: "Ben Ito", Kind: kindCommitmentOurs, Status: "done", Body: sessionPromise, DueAt: &due},
		"their promise": {ContactName: "Ben Ito", Kind: kindCommitmentTheirs, Status: statusOpen, Body: sessionPromise, DueAt: &due},
	} {
		if got := foldSameClaims([]ClaimIn{base, other}); len(got) != 2 {
			t.Errorf("%s: folded into %+v", name, got)
		}
	}
}

func TestEveryLanguageNamesEveryContact(t *testing.T) {
	langcopytest.Census(t, claimWords)
	claim := ClaimIn{ContactName: "Ana Roth", AlsoTo: []string{"Ben Ito", "Cleo Marx"}}
	for _, language := range []string{"en", "de", "vi"} {
		whom := briefCopyFor(language).whom(claim)
		for _, name := range claim.contactNames() {
			if !strings.Contains(whom, name) {
				t.Errorf("%s: %q leaves out %s", language, whom, name)
			}
		}
	}
}
