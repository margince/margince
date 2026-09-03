// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// What the weekly message actually says.
//
// The body is the one part of this arc a rep reads OUTSIDE the product, where
// nothing can be clicked to check it and no panel is beside it to correct it.
// So the shape is asserted here rather than eyeballed once.

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// english is the language most cases here read in, so a case about the SHAPE
// of a message is not also a case about which language it is in.
var english = mailcopy.For(string(mailcopy.English))

func mailFixture() Review {
	amount := int64(1250000)
	return Review{
		LocalWeekStart: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Counts: Counts{
			TasksDue: 9, TasksDone: 7, TasksCarriedOver: 2,
			DealsMoved: 3, DealsWon: 1, DealsLost: 1,
			ProposalsAccepted: 4, ProposalsRejected: 2,
			BriefItemsActed: 6, BriefItemsDismissed: 3,
		},
		Deals: []DealLine{{
			Label: "Stahlbau Krämer", Outcome: OutcomeWon,
			AmountMinor: &amount, Currency: "EUR",
			OccurredAt: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC),
		}},
	}
}

// The counts a rep reads on the screen are the counts the message states. Both
// render from one Review, so this test is what holds them to it: a message
// disagreeing with the panel about somebody's own week gives them no way to
// tell which one lied.
func TestTheMessageStatesTheWeeksNumbers(t *testing.T) {
	body := MailBody(mailFixture(), "https://crm.example.test", english)

	for _, want := range []string{
		"7 of 9", // promised, delivered
		"1 · 1 · 3",
		"4 yes · 2 no", // proposals decided
		"6 acted · 3 dismissed",
		"Carried over",
		"Stahlbau Krämer",
		"12500.00 EUR",
		"2026-06-03",
		"https://crm.example.test",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the weekly message does not state %q:\n%s", want, body)
		}
	}
}

// The subject names the week. These land weekly into a mailbox that already
// holds last week's, and two identical subjects are two messages a reader
// cannot tell apart in a list.
func TestTheSubjectNamesTheWeek(t *testing.T) {
	got := MailSubject(mailFixture(), english)
	if !strings.Contains(got, "2026-06-01") {
		t.Errorf("the subject does not name the week: %q", got)
	}
}

// An installation with no canonical origin mails the week without a link,
// never a link built on an empty base. An unusable URL in a message whose only
// call to action it is would be worse than no line at all.
func TestNoBaseURLMeansNoLinkRatherThanABrokenOne(t *testing.T) {
	body := MailBody(mailFixture(), "", english)
	if strings.Contains(body, "http") {
		t.Errorf("the message carries a link with no base configured:\n%s", body)
	}
	if !strings.Contains(body, "7 of 9") {
		t.Error("the message lost its numbers along with its link")
	}
}

// A deal label is the one field in this body that carries text somebody
// outside the installation may have chosen. Newlines in it must not be able to
// forge structure — a label holding "\n\nFrom: " would otherwise read as a
// header block to anything parsing the message.
func TestADealLabelCannotForgeStructureInTheBody(t *testing.T) {
	review := mailFixture()
	review.Deals[0].Label = "Krämer\r\nFrom: attacker@example.test\n\nYour week of forever"

	body := MailBody(review, "", english)
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "From:") {
			t.Fatalf("a deal label forged a header line:\n%s", body)
		}
	}
	if !strings.Contains(body, "Krämer From: attacker@example.test") {
		t.Errorf("the label was not flattened onto its own line:\n%s", body)
	}
}

// A STAGE NAME is the same species of input as the deal label beside it, and it
// is the one the first version of this file missed. Stage names are stored with
// no single-line validation, so a stage called with a newline reaches the body
// exactly as a hostile label would.
func TestAStageNameCannotForgeStructureInTheBody(t *testing.T) {
	review := mailFixture()
	review.Deals[0].Outcome = OutcomeMoved
	review.Deals[0].ToStageLabel = "Angebot\nDeals:                99 won · 0 lost"

	body := MailBody(review, "", english)
	// The flattened text may still CONTAIN the words; what it must not do is
	// begin a line, which is what makes a forged line read as ours.
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "Deals:") && !strings.Contains(line, "1 won") {
			t.Fatalf("a stage name started a counts line of its own:\n%s", body)
		}
	}
	if !strings.Contains(body, "Angebot Deals:                99 won · 0 lost") {
		t.Errorf("the stage name was not flattened onto the deal's line:\n%s", body)
	}
}

// The MODEL's own sentence is the third, and the most exposed: it is the one
// string in this message a hostile party can steer a generator into writing,
// and narrative.Parse only trims the ends.
func TestTheNarrativeCannotForgeStructureInTheBody(t *testing.T) {
	review := mailFixture()
	review.Narrative = "A quiet week.\n\nCarried into Monday:  0\nFrom: nobody@example.test"

	body := MailBody(review, "", english)
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "From:") {
			t.Fatalf("the narrative forged a header line:\n%s", body)
		}
	}
	// As with a stage name: the words may survive inline, but they must not be
	// able to START a line, which is what makes a forged line read as ours.
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "Carried into Monday:") && !strings.HasSuffix(line, "2") {
			t.Fatalf("the narrative started a carried-over line of its own:\n%s", body)
		}
	}
	if !strings.Contains(body, "A quiet week. Carried into Monday:  0 From: nobody@example.test") {
		t.Errorf("the narrative was not flattened onto one line:\n%s", body)
	}
}

// A week with many deals says how many it did not list. A message that quietly
// stopped at ten would read as a complete week that happened to have ten.
func TestALongWeekSaysWhatItDidNotList(t *testing.T) {
	review := mailFixture()
	// The fixture already carries one, so this brings the week to cap+4.
	for i := 0; i < mailDealCap+3; i++ {
		review.Deals = append(review.Deals, DealLine{
			Label: "Deal", Outcome: OutcomeMoved, OccurredAt: review.LocalWeekStart,
		})
	}

	body := MailBody(review, "", english)
	if !strings.Contains(body, "… and 4 more, on Home") {
		t.Errorf("a capped week does not say what it left out:\n%s", body)
	}
	if got := strings.Count(body, "  · "); got != mailDealCap {
		t.Errorf("the message listed %d deals, the cap is %d", got, mailDealCap)
	}
}

// TestTheMessageIsWrittenInTheInstallationsLanguage is the whole point of the
// catalog: a rep reads their Home panel in German and then this summary of the
// same numbers, and an English message is the product changing language on its
// way out of the browser.
//
// The LABELS are asserted, not the numbers — the numbers are the same in every
// language, so a case that only read them would pass on an English message.
func TestTheMessageIsWrittenInTheInstallationsLanguage(t *testing.T) {
	for _, c := range []struct {
		language string
		subject  string
		labels   []string
	}{
		{
			language: "de",
			subject:  "Deine Woche",
			labels:   []string{"Zugesagt, erledigt", "7 von 9", "Von dir entschieden", "Morgen-Liste", "Übernommen", "gewonnen"},
		},
		{
			language: "vi",
			subject:  "Tuần của bạn",
			labels:   []string{"Đã hứa, đã xong", "7 trên 9", "Bạn đã quyết", "Danh sách buổi sáng", "Chuyển tiếp", "thắng"},
		},
		{
			// A language this build has no copy for is written in the fallback
			// rather than refused: a summary in the wrong language is worth
			// more to its reader than no summary.
			language: "fr",
			subject:  "Your week",
			labels:   []string{"Promised, delivered", "You decided", "Morning queue", "Carried over", "won"},
		},
	} {
		t.Run(c.language, func(t *testing.T) {
			words := mailcopy.For(c.language)
			if got := MailSubject(mailFixture(), words); !strings.Contains(got, c.subject) {
				t.Errorf("the subject is %q, want it to carry %q", got, c.subject)
			}
			body := MailBody(mailFixture(), "", words)
			for _, label := range c.labels {
				if !strings.Contains(body, label) {
					t.Errorf("the message does not carry %q:\n%s", label, body)
				}
			}
		})
	}
}

// TestTheLabelColumnIsSizedToTheLabelsItHas holds the layout in a language
// whose words are longer than the English ones it was laid out for.
//
// Padding by BYTES rather than runes short-changes every row with an umlaut or
// a Vietnamese diacritic in it, which is a ragged column in two of the three
// languages and a straight one in the language nobody needed this for.
func TestTheLabelColumnIsSizedToTheLabelsItHas(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		t.Run(language, func(t *testing.T) {
			body := MailBody(mailFixture(), "", mailcopy.For(language))
			widths := map[int]bool{}
			for _, line := range strings.Split(body, "\n") {
				colon := strings.Index(line, ":")
				// Only the tally rows: they are the ones laid out in columns,
				// and they are what a ragged edge would show in.
				if colon <= 0 || strings.HasPrefix(line, " ") || strings.Contains(line, "http") {
					continue
				}
				// A line with nothing after its colon is a heading, not a
				// tally: "What moved:" ends one section and starts another.
				if strings.TrimSpace(line[colon+1:]) == "" {
					continue
				}
				widths[utf8.RuneCountInString(line[:colon+1])+countLeadingSpaces(line[colon+1:])] = true
			}
			if len(widths) > 1 {
				t.Errorf("the tally rows start their values at %d different columns:\n%s", len(widths), body)
			}
		})
	}
}

// countLeadingSpaces is how far a value sits from its label's colon.
func countLeadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

// The link opens the WEEK, not the morning.
//
// The app is one hash-routed page, so the view a link means lives after the
// '#'. This message sent the reader to the bare origin, which opens the Brief
// on its default view — so a Monday summary of last week landed them on today's
// queue, with nothing on either page saying they had been sent to the wrong one.
//
// Asserted as the WHOLE closing address rather than as a substring: the origin
// alone appears in both the right answer and the wrong one, which is why the
// test above passed either way.
func TestTheWeeklyLinkOpensTheWeek(t *testing.T) {
	body := MailBody(mailFixture(), "https://crm.example.test", english)

	const want = "https://crm.example.test/#/home?view=weekly"
	if !strings.Contains(body, want) {
		t.Errorf("the weekly message does not link to %q:\n%s", want, body)
	}
}

// A trailing slash on the configured origin does not become a double one.
func TestTheWeeklyLinkSurvivesATrailingSlash(t *testing.T) {
	body := MailBody(mailFixture(), "https://crm.example.test/", english)

	if strings.Contains(body, "test//#/") {
		t.Errorf("the weekly link doubled the separator:\n%s", body)
	}
}

// No origin, no line. An installation that has not been told its own public
// address cannot produce a working link, and a label over an empty indent reads
// as one that failed to render.
func TestTheWeeklyOmitsTheLinkLineWithNoOrigin(t *testing.T) {
	body := MailBody(mailFixture(), "", english)

	if strings.Contains(body, english.WeeklyFullWeek) {
		t.Errorf("the weekly wrote its link label with no address under it:\n%s", body)
	}
}
