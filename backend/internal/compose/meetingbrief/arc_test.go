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

// A conversation this caller may not read says nothing and must not date the
// arc either. The arc's own `omitted` line tells the reader "the account arc
// is built from the rest"; a withheld conversation's own instant reaching
// plan.account_arc[].to (or its count) makes that sentence false, and handed
// the reader the withheld message's date by subtraction.
func TestAWithheldConversationNamesNothing(t *testing.T) {
	in := fullInput()
	withheld := mail(dealID, 5, "", "inbound")
	withheld.Withheld = true
	in.History = []HistoryIn{
		mail(activityID, 3, "Readable subject", "inbound"),
		withheld,
	}
	arc := accountArc(in)
	for _, moment := range arc {
		if strings.Contains(moment.Title, "withheld") {
			t.Errorf("a withheld conversation named itself in the arc title %q", moment.Title)
		}
	}
	if len(arc) != 1 {
		t.Fatalf("moments = %d, want 1 — the withheld-only thread has its own moment, dropped", len(arc))
	}
	if got, want := arc[0].To, at(3); !got.Equal(want) {
		t.Errorf("moment.To = %s, want %s — the withheld message's own date must not extend it", got, want)
	}
	if got := conversationCount(arc[0]); got != 1 {
		t.Errorf("conversation count = %d, want 1 — the withheld conversation is not this reader's to count", got)
	}
	if got := withheldCount(in.History); got != 1 {
		t.Errorf("withheld count = %d, want 1 — the omission is built from it", got)
	}
}

// A withheld row inside a READABLE thread is the case the audience narrowing
// is for. The thread is cited, and the withheld row must not be one of the
// citations — sending a reader to a record they cannot open also tells them a
// limited conversation belongs to this thread.
func TestAWithheldRowIsNotCitedEvenInsideAReadableThread(t *testing.T) {
	readable := mail(activityID, 3, "Requirements", "inbound")
	readable.ThreadKey = "abc"
	hidden := mail(dealID, 4, "", "outbound")
	hidden.ThreadKey = "abc"
	hidden.Withheld = true

	threads := threadsOf([]HistoryIn{readable, hidden})
	if len(threads) != 1 {
		t.Fatalf("threads = %d, want 1 — one thread key is one thread", len(threads))
	}
	for _, id := range threads[0].IDs {
		if id == dealID {
			t.Error("a withheld row is citable through the thread it shares with a readable one")
		}
	}
	// It still COUNTS in Rows — a withheld row still happened — but must not
	// move First/Last: the hidden reply landed on day 4, a day later than the
	// readable message it replies to, and the thread's dates must stay at the
	// readable message's own day, not stretch to the hidden reply's.
	if threads[0].Rows != 2 {
		t.Errorf("rows = %d, want 2 — a withheld row still happened", threads[0].Rows)
	}
	if got, want := threads[0].Last, at(3); !got.Equal(want) {
		t.Errorf("last = %s, want %s — a withheld reply must not date the thread", got, want)
	}
}

// The history reader hands threadsOf rows newest first (history.go's
// ORDER BY occurred_at DESC), the opposite order the test above uses. The fix
// must hold either way: the guard is "was this row readable", not "was it the
// first one seen for this key".
func TestAWithheldRowIsNotCitedRegardlessOfScanOrder(t *testing.T) {
	readable := mail(activityID, 3, "Requirements", "inbound")
	readable.ThreadKey = "abc"
	hidden := mail(dealID, 4, "", "outbound")
	hidden.ThreadKey = "abc"
	hidden.Withheld = true

	threads := threadsOf([]HistoryIn{hidden, readable})
	if len(threads) != 1 {
		t.Fatalf("threads = %d, want 1 — one thread key is one thread", len(threads))
	}
	for _, id := range threads[0].IDs {
		if id == dealID {
			t.Error("a withheld row is citable through the thread it shares with a readable one")
		}
	}
	if got, want := threads[0].Last, at(3); !got.Equal(want) {
		t.Errorf("last = %s, want %s — a withheld reply scanned FIRST must still not date the thread", got, want)
	}
}

// A withheld row's own nature — that it was a meeting, was on the deal, got
// a reply — must not bias which readable moment ranks highest or which
// readable subject becomes a moment's title. Either would let hidden content
// steer what the reader is shown through a channel the audience narrowing
// cannot see.
func TestAWithheldRowsNatureDoesNotBiasRankingOrTitle(t *testing.T) {
	plain := mail(activityID, 3, "Ordinary note", "outbound")
	plain.ThreadKey = "abc"
	loudButHidden := HistoryIn{
		ID: dealID, Kind: "meeting", Subject: "Should never be the title",
		Direction: "inbound", At: at(4), OnDeal: true, Withheld: true,
	}
	loudButHidden.ThreadKey = "abc"

	threads := threadsOf([]HistoryIn{plain, loudButHidden})
	if len(threads) != 1 {
		t.Fatalf("threads = %d, want 1", len(threads))
	}
	got := threads[0]
	if got.HasMeeting {
		t.Error("a withheld meeting made the thread read as a meeting")
	}
	if got.OnDeal {
		t.Error("a withheld on-deal row made the thread read as on-deal")
	}
	if got.Inbound != 0 {
		t.Errorf("inbound = %d, want 0 — the only inbound row is withheld", got.Inbound)
	}
	if got.Subject != "Ordinary note" {
		t.Errorf("subject = %q, want the readable row's own subject, not the withheld row's louder one", got.Subject)
	}
}

// An all-withheld moment must never outrank, and so never displace, a moment
// built from anything this caller may read — not even when the withheld
// content looks maximally important (a meeting, on the deal, replied to).
// Six moments compete for the five-slot cap; the sixth, weakest readable one
// must still beat the loud-but-hidden moment, proving the floor holds under
// real competition rather than only in an uncapped fixture.
func TestALoudWithheldMomentNeverDisplacesAWeakReadableOne(t *testing.T) {
	in := fullInput()
	var history []HistoryIn
	loud := HistoryIn{
		ID: dealID, Kind: "meeting", Subject: "Loud but hidden",
		Direction: "inbound", At: at(1), OnDeal: true, Withheld: true,
	}
	history = append(history, loud)
	// Six ordinary, weak, single-message moments — no deal, no meeting, no
	// reply, nothing to score beyond recency — each a silence-closing gap
	// apart so they never merge into one.
	for i := range arcCap + 1 {
		history = append(history, mail(
			fmt.Sprintf("0198f000-0000-7000-8000-0000000%05d", i+400),
			40+i*30, fmt.Sprintf("Weak %d", i), "outbound"))
	}
	in.History = history
	in.Now = at(40 + arcCap*30)

	arc := accountArc(in)
	if len(arc) != arcCap {
		t.Fatalf("moments = %d, want %d — the cap, filled entirely from readable moments", len(arc), arcCap)
	}
	for _, moment := range arc {
		if strings.Contains(moment.Title, "Loud but hidden") {
			t.Fatal("the withheld moment's title reached the reader")
		}
		for _, current := range moment.Threads {
			if current.HasMeeting && len(current.IDs) == 0 {
				t.Fatal("the withheld moment occupied a slot in the capped arc")
			}
		}
	}
}

// The fold exists so a reply chain reads as one conversation. Reporting its
// message count would undo it in the one sentence a reader actually sees.
func TestAMomentCountsConversationsNotMessages(t *testing.T) {
	in := fullInput()
	var chain []HistoryIn
	for i := range 12 {
		row := mail(fmt.Sprintf("0198f000-0000-7000-8000-0000000%05d", i+300),
			3+i, "Re: CRM requirements", "inbound")
		row.ThreadKey = "one-thread"
		chain = append(chain, row)
	}
	in.History = chain
	arc := accountArc(in)
	if len(arc) != 1 {
		t.Fatalf("moments = %d, want 1", len(arc))
	}
	if got := conversationCount(arc[0]); got != 1 {
		t.Errorf("conversation count = %d, want 1 — twelve replies are one conversation", got)
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
