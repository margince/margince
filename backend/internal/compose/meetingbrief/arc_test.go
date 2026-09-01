// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func mail(id string, day int, subject, direction string) HistoryIn {
	return HistoryIn{
		ID: id, Kind: "email", Subject: subject, Direction: direction, At: at(day),
	}
}

// A reply keeps the subject and adds a prefix to it. Counting a twelve-message
// argument as twelve conversations is how a ranked history fills with the
// loudest week instead of the most important one.
func TestAReplyChainIsOneConversation(t *testing.T) {
	history := []HistoryIn{
		mail(activityID, 3, "CRM requirements", "outbound"),
		mail(dealID, 4, "Re: CRM requirements", "inbound"),
		mail(personID, 5, "AW: Re: CRM requirements", "outbound"),
		mail(projectID, 6, "WG: AW: Re: CRM requirements", "inbound"),
	}
	threads := threadsOf(history)
	if len(threads) != 1 {
		t.Fatalf("threads = %d, want 1 — the prefixes are the same conversation", len(threads))
	}
	if got := threads[0].Inbound; got != 2 {
		t.Errorf("inbound = %d, want 2", got)
	}
}

// A thread key written by the capture outranks the subject, because a subject
// can be edited mid-thread and the key cannot.
func TestTheCapturesThreadKeyDecidesBeforeTheSubject(t *testing.T) {
	first := mail(activityID, 3, "Requirements", "inbound")
	first.ThreadKey = "abc"
	second := mail(dealID, 4, "Something else entirely", "outbound")
	second.ThreadKey = "abc"
	if got := threadsOf([]HistoryIn{first, second}); len(got) != 1 {
		t.Fatalf("threads = %d, want 1 — one thread key is one thread", len(got))
	}
}

// Merging every subjectless note into one bucket would invent a conversation
// nobody had.
func TestNotesWithNoSubjectNeverMerge(t *testing.T) {
	history := []HistoryIn{
		{ID: activityID, Kind: "note", At: at(3)},
		{ID: dealID, Kind: "note", At: at(4)},
	}
	if got := threadsOf(history); len(got) != 2 {
		t.Fatalf("threads = %d, want 2 — two unnamed notes are not one conversation", len(got))
	}
}

// A silence is how a relationship punctuates itself.
func TestALongSilenceSplitsTheArcIntoMoments(t *testing.T) {
	history := []HistoryIn{
		mail(activityID, 1, "Scoping", "inbound"),
		mail(dealID, 3, "Re: Scoping", "outbound"),
		// Well past arcGapDays.
		mail(personID, 28, "New question", "inbound"),
	}
	moments := clusterThreads(threadsOf(history))
	if len(moments) != 2 {
		t.Fatalf("moments = %d, want 2 — a three-week gap closes one", len(moments))
	}
}

// THE test that says the arc is worth having. The old reader took the ten
// newest rows; this one must keep the moment where something was AGREED even
// when a pile of newer chatter would have buried it.
func TestAMomentHoldingAPromiseOutranksNewerChatter(t *testing.T) {
	in := fullInput()
	// The promise was made here, months ago.
	in.Commitments = []ClaimIn{{
		PersonName: "Ana Roth", Kind: kindCommitmentOurs, Body: "send the security pack",
		Status: statusOpen, SourceID: activityID,
	}}
	history := []HistoryIn{mail(activityID, 1, "Security review", "inbound")}
	// Eight later bursts, each separated from its neighbours by more than
	// arcGapDays so they become their own moments and compete for the five
	// slots. They are deliberately STRONG competition — two-way, on the deal,
	// and far more recent — so that only the promise's own weight keeps its
	// moment in. Weak chatter would let the test pass with the ranking turned
	// off, which is a test that proves nothing.
	for burst := range 8 {
		day := 40 + burst*30
		for message := range 5 {
			row := mail(
				fmt.Sprintf("0198f000-0000-7000-8000-%012d", burst*10+message+100),
				day+message, fmt.Sprintf("Chatter %d-%d", burst, message), "inbound")
			row.OnDeal = true
			history = append(history, row)
		}
	}
	in.History = history
	in.Now = at(300)

	arc := accountArc(in)
	if len(arc) == 0 {
		t.Fatal("the arc is empty")
	}
	for _, moment := range arc {
		for _, current := range moment.Threads {
			for _, id := range current.IDs {
				if id == activityID {
					return
				}
			}
		}
	}
	t.Errorf("the moment holding the open promise was ranked out by newer chatter; arc = %d moments", len(arc))
}

// The arc is read forwards whatever the ranking decided.
func TestTheArcReadsInDateOrder(t *testing.T) {
	in := fullInput()
	var history []HistoryIn
	for i := range 12 {
		history = append(history, mail(
			fmt.Sprintf("0198f000-0000-7000-8000-0000000%05d", i+200),
			1+i*4, fmt.Sprintf("Thread %d", i), "inbound"))
	}
	in.History = history
	in.Now = at(60)

	arc := accountArc(in)
	if len(arc) > arcCap {
		t.Fatalf("arc = %d moments, want at most %d", len(arc), arcCap)
	}
	var last time.Time
	for _, moment := range arc {
		if !last.IsZero() && moment.From.Before(last) {
			t.Fatalf("moment starting %s follows one starting %s — the arc is out of order",
				moment.From, last)
		}
		last = moment.From
	}
}

// A conversation this caller may not read still dates the arc and says
// nothing. Its subject must not reach the reader by any path.
func TestAWithheldConversationNamesNothing(t *testing.T) {
	in := fullInput()
	withheld := mail(dealID, 5, "", "inbound")
	withheld.Withheld = true
	in.History = []HistoryIn{
		mail(activityID, 3, "Readable subject", "inbound"),
		withheld,
	}
	for _, moment := range accountArc(in) {
		if strings.Contains(moment.Title, "withheld") {
			t.Errorf("a withheld conversation named itself in the arc title %q", moment.Title)
		}
	}
	if got := withheldCount(in.History); got != 1 {
		t.Errorf("withheld count = %d, want 1 — the omission is built from it", got)
	}
}

// A moment nobody may read is dropped rather than titled: it has no subject to
// name and no citation the reader could open.
func TestAMomentNobodyMayReadIsNotShown(t *testing.T) {
	in := fullInput()
	only := mail(dealID, 5, "", "inbound")
	only.Withheld = true
	in.History = []HistoryIn{only}
	if got := accountArc(in); len(got) != 0 {
		t.Errorf("arc = %d moments, want 0 — every conversation in it is withheld", len(got))
	}
}
