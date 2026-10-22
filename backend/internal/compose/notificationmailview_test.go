// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

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

// A commitment's email names what the card names, one fact a line, and never
// repeats its subject as the heading.
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
	if n := strings.Count(message.Text, notice.Subject); n != 1 {
		t.Errorf("the subject appears %d times, want once, as the subject line", n)
	}
	if strings.Contains(message.Text, long) {
		t.Error("a long quote was not cut")
	}

	bare := notificationMessage(notice, approvalMailFacts{Rich: true, Deadline: facts.Deadline}, "https://example.test", words)
	if strings.Count(bare.Text, notice.Subject) != 1 || !strings.Contains(bare.Text, "Decision deadline: tomorrow") {
		t.Errorf("an email with no facts must have no heading and still its deadline: %q", bare.Text)
	}
}
