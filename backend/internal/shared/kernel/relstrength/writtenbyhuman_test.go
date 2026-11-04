// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import (
	"strings"
	"testing"
)

func TestAutomaticNamesWhatSentARowNoHumanWrote(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  Evidence
		want AutomaticKind
	}{
		{"a human's mail", Evidence{Kind: "email", Origin: "human"}, ""},
		{"a calendar invitation", Evidence{Kind: "email", Origin: "human", CalendarMethod: "REQUEST"}, AutomaticCalendar},
		{"a reply to an invitation", Evidence{Kind: "email", Origin: "human", CalendarMethod: "REPLY"}, AutomaticCalendar},
		{"a file-share notice", Evidence{Kind: "email", Origin: "human", SenderIsMachine: true}, AutomaticSendingSystem},
		{"the product's own mail", Evidence{Kind: "email", Origin: "system_notice"}, AutomaticProduct},
		{"work the product filed", Evidence{Kind: "task", Origin: "system_remediation"}, AutomaticProduct},
		{"a logged call", Evidence{Kind: "call", Origin: "human"}, ""},
		{"a meeting with a calendar method stays a meeting", Evidence{Kind: "meeting", Origin: "human", CalendarMethod: "REQUEST"}, ""},
		{"an agent's mail", Evidence{Kind: "email", Origin: "agent"}, ""},
		{"a newsletter", Evidence{Kind: "email", Origin: "human", BulkMail: true}, AutomaticBulk},
		{"a notice with an unsubscribe header is a sending system's", Evidence{Kind: "email", Origin: "human", SenderIsMachine: true, BulkMail: true}, AutomaticSendingSystem},
		{"a meeting with the bulk stamp stays a meeting", Evidence{Kind: "meeting", Origin: "human", BulkMail: true}, ""},
	}
	for _, c := range cases {
		if got := c.row.Automatic(); got != c.want {
			t.Errorf("%s: Automatic() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAnAutomaticRowIsNoInteraction(t *testing.T) {
	t.Parallel()
	for _, kind := range AutomaticKinds() {
		if InteractionCounts("email", "", "", kind) {
			t.Errorf("a %s mail counts as an interaction", kind)
		}
	}
	if !InteractionCounts("email", "", "", "") {
		t.Error("a human's mail must count as an interaction")
	}
}

// Each fact Automatic reads is a clause of the SQL twin. The integration lane
// evaluates both over the same rows (TestTheGoAndSQLWrittenByHumanRulesAgree).
func TestTheCountingRuleAsksWhoWroteTheRow(t *testing.T) {
	t.Parallel()
	rule := InteractionCountsSQL("a")
	for _, clause := range []string{"NOT a.sender_is_machine", "a.calendar_method IS NULL", "NOT a.bulk_mail_attested", OriginIsEngagementSQL("a")} {
		if !strings.Contains(rule, clause) {
			t.Errorf("InteractionCountsSQL = %q lacks %q", rule, clause)
		}
	}
	for _, origin := range systemOrigins {
		if !strings.Contains(OriginIsEngagementSQL("a"), "'"+origin+"'") {
			t.Errorf("OriginIsEngagementSQL does not exclude %q", origin)
		}
	}
}
