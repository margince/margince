// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func TestNoticeWordsCannotForgeMailStructure(t *testing.T) {
	notice := notices.Notice{Subject: "Quota reached\nFrom: finance@example.test", Body: "First line\nSubject: another message"}
	words := mailcopy.For("en")
	subject := notificationMailSubject(notice, words)
	body := notificationMessage(notice, approvalMailFacts{}, "https://example.test", words).Text
	if strings.ContainsAny(subject, "\r\n") {
		t.Fatal("subject contains an injected header")
	}
	for _, line := range []string{"\nFrom:", "\nSubject:"} {
		if strings.Contains(body, line) {
			t.Fatal("notice forged a structural line")
		}
	}
	if !strings.Contains(body, "Quota reached") || !strings.Contains(body, "First line") {
		t.Fatal("flattening lost the actual content")
	}
}

// A commitment's email names what the card names, one fact a line, and states
// the notice's sentence once, under the heading.
func TestACommitmentEmailNamesItsFactsOneALine(t *testing.T) {
	words := mailcopy.For("en")
	notice := notices.Notice{Subject: "Send the revised pricing", Body: "Decision deadline: soon"}
	long := strings.Repeat("word ", 200)
	facts := approvalMailFacts{
		Rich: true, Heading: "Fleet renewal\nFrom: forged@example.test",
		Lines: []approvalMailLine{
			{Label: words.MailAbout, Value: "Fleet renewal"},
			{Label: words.MailPromisedBy, Value: "Lena\nSubject: forged"},
			{Label: words.MailInTheirWords, Value: long},
		},
		Deadline: words.MailDeadline + ": tomorrow",
	}
	message := notificationMessage(notice, facts, "https://example.test", words)
	for _, line := range []string{"\nFrom:", "\nSubject:"} {
		if strings.Contains(message.Text, line) {
			t.Fatalf("a fact forged a structural line: %q", message.Text)
		}
	}
	for _, want := range []string{"Fleet renewal", "Promised by: Lena", "Decision deadline: tomorrow"} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("the email does not say %q: %q", want, message.Text)
		}
	}
	if n := strings.Count(message.Text, notice.Subject); n != 2 {
		t.Errorf("the sentence appears %d times, want twice: subject line and body", n)
	}
	if strings.Contains(message.Text, long) {
		t.Error("a long quote was not cut")
	}

	bare := notificationMessage(notice, approvalMailFacts{Rich: true, Deadline: facts.Deadline}, "https://example.test", words)
	if strings.Count(bare.Text, notice.Subject) != 2 || !strings.Contains(bare.Text, "Decision deadline: tomorrow") {
		t.Errorf("an email with no facts must still have its sentence and deadline: %q", bare.Text)
	}
}

// A phone shows about sixty characters of a subject. The body keeps what the cut drops.
func TestALongApprovalSubjectEndsOnAWholeWordAndTheBodyKeepsItAll(t *testing.T) {
	summary := `Set up a partner program on company "ZZ email approval company mwg26xsk" (kinds: referral), with no margin tier`
	notice := notices.Notice{Subject: summary, Body: "Decision deadline: soon"}
	for _, language := range []string{"en", "de", "vi"} {
		words := mailcopy.For(language)
		subject := notificationMailSubject(notice, words)
		if n := utf8.RuneCountInString(subject); n > 100 {
			t.Errorf("%s: subject is %d characters: %q", language, n, subject)
		}
		kept, cut := strings.CutSuffix(strings.TrimPrefix(subject, words.NotificationSubject), "…")
		if !cut || !strings.HasPrefix(summary, kept) || summary[len(kept)] != ' ' {
			t.Errorf("%s: %q does not end on a whole word of the summary", language, subject)
		}
		if body := notificationMessage(notice, approvalMailFacts{}, "https://example.test", words).Text; !strings.Contains(body, summary) {
			t.Errorf("%s: the body lost the full sentence", language)
		}
	}
	short := notices.Notice{Subject: "Set up a partner program"}
	if got := notificationMailSubject(short, mailcopy.For("en")); got != "Your decision is needed: Set up a partner program" {
		t.Errorf("a short subject changed: %q", got)
	}
}

// The rich body has no other place for the sentence a cut subject drops.
func TestARichApprovalEmailWithALongSubjectCarriesTheFullSentence(t *testing.T) {
	summary := `Set up a partner program on company "ZZ email approval company mwg26xsk" (kinds: referral), with no margin tier`
	notice := notices.Notice{Subject: summary}
	words := mailcopy.For("en")
	facts := approvalMailFacts{
		Rich: true, Heading: "ZZ email approval company",
		Lines:    []approvalMailLine{{Label: words.MailAbout, Value: "Partner program"}},
		Deadline: words.MailDeadline + ": tomorrow",
	}
	text := notificationMessage(notice, facts, "https://example.test", words).Text
	if strings.Contains(notificationMailSubject(notice, words), summary) {
		t.Fatal("the subject was not cut, so the test proves nothing")
	}
	order := []string{"ZZ email approval company\n", summary, "About: Partner program", "Decision deadline: tomorrow"}
	last := -1
	for _, want := range order {
		at := strings.Index(text, want)
		if at <= last {
			t.Fatalf("%q is missing or out of order in %q", want, text)
		}
		last = at
	}
}
