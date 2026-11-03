// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"
	"testing"
)

// The last conversation line names who wrote it, which the row's voice answers
// where it names a side. A colleague's mail arrives inbound, so the transport
// alone hands the counterpart a message we sent.
func TestTheLastConversationLineNamesTheVoiceNotTheTransport(t *testing.T) {
	said := briefCopyFor("en")
	for _, tc := range []struct {
		name string
		act  ActIn
		want string
	}{
		{"a colleague's inbound mail is ours", ActIn{
			Subject: "Prices", Direction: "inbound", WroteOurSide: true, SideKnown: true,
		}, said.Say(sectionWords.WeWrote)},
		{"the counterpart's mail is theirs", ActIn{
			Subject: "Prices", Direction: "inbound", WroteOurSide: false, SideKnown: true,
		}, said.Say(sectionWords.TheyWrote)},
		{"a row naming no side is neither", ActIn{
			Subject: "Prices", SideKnown: false,
		}, said.Say(sectionWords.LastCaptured)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := said.lastConversationLine(tc.act)
			if want := fmt.Sprintf(tc.want, said.subjectOrKind(tc.act)); got != want {
				t.Errorf("line = %q, want %q", got, want)
			}
		})
	}
}
