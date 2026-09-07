// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// rosterFrom builds the record one document with a roster lands as, failing the
// test rather than the assertion when the document is not landable at all.
func rosterFrom(tb testing.TB, doc arrival) extension.Record {
	tb.Helper()
	rec, err := recordFor(ownerRef, arrivalJSON(tb, doc), signedAt)
	if err != nil {
		tb.Fatalf("building a record: %v", err)
	}
	return rec
}

// A group's roster is the whole point of the field: a sender says who else was
// in the room, and the record carries every one of them through to the core.
func TestARosterReachesTheRecord(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	doc.Participants = []party{
		{Account: "acct-51", Name: "Sam Okonkwo"},
		{Account: "acct-77", Email: "legal@example.net", Name: "Priya Raman"},
	}
	rec := rosterFrom(t, doc)

	if len(rec.Participants) != 2 {
		t.Fatalf("the record carries %d participants, want the two the document named", len(rec.Participants))
	}
	first := rec.Participants[0]
	if first.Account != "acct-51" || first.Name != "Sam Okonkwo" || first.Email != "" {
		t.Errorf("the first party landed as %+v, want the account and name the document gave and no address", first)
	}
	second := rec.Participants[1]
	if second.Account != "acct-77" || second.Email != "legal@example.net" {
		t.Errorf("the second party landed as %+v, want both the account and the address the document gave", second)
	}
	// Everyone is an attendee: this document has no vocabulary for where in a
	// conversation somebody stood, and inventing one would put a header
	// position on a message that has no headers.
	for _, p := range rec.Participants {
		if p.Role != extension.ParticipantRoleAttendee {
			t.Errorf("%q landed in role %q, want %q", p.Account, p.Role, extension.ParticipantRoleAttendee)
		}
	}
	if err := rec.Validate(); err != nil {
		t.Fatalf("the core refuses a record this unit built: %v", err)
	}
}

// A document naming nobody is the ordinary two-party message, and it must reach
// the core exactly as it did before the field existed.
func TestADocumentWithNoRosterCarriesNone(t *testing.T) {
	t.Parallel()
	rec := rosterFrom(t, landableArrival())
	if rec.Participants != nil {
		t.Fatalf("a document naming nobody landed %d participants", len(rec.Participants))
	}
}

// A party the sender described with no account and no address is dropped, and
// the MESSAGE still lands. The core refuses the whole record over such a party,
// so passing it on would lose a message to a sender's typo — one this connector
// can see and the sender cannot.
func TestAPartyWithNoIdentityIsDroppedAndTheMessageLands(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	doc.Participants = []party{
		{Name: "Bob"},
		{Account: "acct-51", Name: "Sam Okonkwo"},
	}
	rec := rosterFrom(t, doc)

	if len(rec.Participants) != 1 {
		t.Fatalf("the record carries %d participants, want only the one that could be identified", len(rec.Participants))
	}
	if rec.Participants[0].Account != "acct-51" {
		t.Errorf("the surviving party is %+v, want the one the document identified", rec.Participants[0])
	}
	if err := rec.Validate(); err != nil {
		t.Fatalf("the core refuses the record the drop was made to save: %v", err)
	}
}

// The roster's addresses stay OFF the record's address set, and this is the
// assertion that catches the tempting mistake. That set is what decides whether
// a message was purely internal — every party on our own domains — so folding a
// group's colleagues into it is how a message from a real outside sender comes
// to look like colleagues talking and gets dropped.
func TestARosterDoesNotJoinTheAddressSet(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	doc.Participants = []party{{Account: "acct-51", Email: "legal@example.net"}}
	rec := rosterFrom(t, doc)

	for _, address := range rec.Addresses {
		if address == "legal@example.net" {
			t.Fatalf("a roster address reached the address set: %v", rec.Addresses)
		}
	}
	// And the set is still exactly what the two ends named, so the drop above is
	// the roster's alone rather than the set having gone empty.
	if len(rec.Addresses) != 2 {
		t.Fatalf("the address set is %v, want the sender's and the member's", rec.Addresses)
	}
}

// A sender who names no address anywhere — the case this connector exists for —
// still gets an empty set with a roster present. The rule is addressesOf's and
// belongs to the SENDER: an account-only sender forces the set empty, which the
// core reads as "could not enumerate the parties" and keeps the record. A roster
// must not quietly re-populate it.
func TestAnAccountOnlySenderKeepsAnEmptySetDespiteARoster(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	doc.From = party{Account: "acct-77", Name: "Ada Buyer"}
	doc.Participants = []party{{Account: "acct-51", Email: "legal@example.net"}}
	rec := rosterFrom(t, doc)

	if len(rec.Addresses) != 0 {
		t.Fatalf("the address set is %v, want empty — the sender named no address", rec.Addresses)
	}
	if err := rec.Validate(); err != nil {
		t.Fatalf("the core refuses an account-only record carrying a roster: %v", err)
	}
}

// Past the cap the core refuses the record, and this unit does not pre-empt it.
// The refusal is what a unit author reads; a silent truncation here would report
// a broadcast list as a small conversation and nothing would fail.
func TestARosterOverTheCapIsRefusedByTheCore(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	for i := 0; i <= extension.MaxParticipants; i++ {
		doc.Participants = append(doc.Participants, party{Account: string(rune('a'+i%26)) + "-acct"})
	}
	rec := rosterFrom(t, doc)
	if err := rec.Validate(); err == nil {
		t.Fatalf("a roster of %d parties passed the cap of %d", len(rec.Participants), extension.MaxParticipants)
	}
}
