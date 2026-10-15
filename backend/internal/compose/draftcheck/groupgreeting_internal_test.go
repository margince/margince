// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// The floor's group greeting passes the greeting checks in every language they
// judge. A draft to several readers is never flagged for the floor's own line.
// The languages come from greetingOpeners, so one added there is held here too.
func TestTheFloorsGroupGreetingPassesInEveryJudgedLanguage(t *testing.T) {
	to := Grounds{FirstName: "Anna", LastName: "Weber", CoRecipients: []string{"Bob Meyer"}}
	for lang := range greetingOpeners {
		body := draftfloor.GroupGreeting(lang) + "\n\nWednesday works."
		if findings := Formatting(body, lang, to); len(findings) > 0 {
			t.Errorf("the %s group greeting %q was flagged: %+v", lang, draftfloor.GroupGreeting(lang), findings)
		}
		envelope := draftfloor.Envelope{Language: string(lang), ConversationState: "fresh"}
		if got := EnsureGreeting(body, envelope, to); got != body {
			t.Errorf("the %s group greeting was rewritten to %q", lang, got)
		}
	}
}
