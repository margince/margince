// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A note stored with the words the catalog held before they said "here" still
// binds to its descriptor, so a reader sees today's words in their language.
func TestAStoredDealNoteShowsTheCurrentWordsInEveryLanguage(t *testing.T) {
	stored := "No reply for 120 days. Undo it from Home if the deal is still alive, and add a note on why."
	body := Sequence(Sentence("deal.customer_declined"), Sentence("deal.lost_undo"))
	body.Layout = "sentences"
	body.Digest = Fingerprint(stored)
	message := &Message{Version: 1, Body: body}
	for lang, want := range map[textlang.Lang]string{textlang.German: "hier rückgängig", textlang.Vietnamese: "tại đây"} {
		_, body := message.Displays("x", stored, lang)
		if body == nil || !strings.Contains(*body, want) || strings.Contains(*body, "Startseite") {
			t.Errorf("%s shows %v, want the words %q", lang, body, want)
		}
	}
}
