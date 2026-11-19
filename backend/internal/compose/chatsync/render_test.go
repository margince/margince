// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
)

// A record name, a subject or an owner's display name is attacker-controlled
// text. It must not forge a broadcast, a mention or a link in the bot's voice.
func TestChatTextNeutralisesRecordMarkup(t *testing.T) {
	record := chatRecord{
		kind:       "deal",
		name:       "@channel <@all> Renewal",
		stage:      "Negotiation",
		ownerEmail: "rep@firm.test",
		ownerName:  "<@owner> Boss",
	}
	for name, got := range map[string]string{
		"card": chatCardText(record, "https://app.test"),
		"post": chatPostText(record, []string{"a line"}, "https://app.test"),
	} {
		if strings.Contains(got, "@channel") {
			t.Errorf("%s left a live @channel broadcast: %q", name, got)
		}
		if strings.Contains(got, "<@all>") {
			t.Errorf("%s left a forged mention from the record name: %q", name, got)
		}
		if strings.Contains(got, "<@owner>") {
			t.Errorf("%s left a forged mention from the owner name: %q", name, got)
		}
	}
}

// An activity subject rides the same path and is cut before it is escaped. A
// captured email from an outside sender cannot then broadcast or forge a link.
func TestActivityLineNeutralisesAndCapsTheSubject(t *testing.T) {
	line := activityLine(chatlinks.Item{EventType: "activity.created"},
		sharedText{kind: "email", subject: "@everyone see <https://evil.example|click>"})
	if strings.Contains(line, "@everyone") {
		t.Errorf("activity line left a live @everyone broadcast: %q", line)
	}
	if strings.Contains(line, "<https://evil.example") {
		t.Errorf("activity line left a forged link: %q", line)
	}

	long := activityLine(chatlinks.Item{EventType: "activity.created"},
		sharedText{kind: "note", subject: strings.Repeat("x", 500)})
	if n := utf8.RuneCountInString(long); n > maxSubjectRunes+50 {
		t.Errorf("activity line did not cap a 500-rune subject: %d runes", n)
	}
}
