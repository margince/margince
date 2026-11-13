// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/chatsummary"
)

func TestAMessageIsNamedBySeqMentionOrLinkInItsOwnChannelOnly(t *testing.T) {
	for _, tc := range []struct {
		ref  string
		want int64
		ok   bool
	}{
		{"42", 42, true},
		{"#acme:42", 42, true},
		{"<m:#acme:42|this message>", 42, true},
		{"<m:#acme:40:42|this reply>", 40, true},
		{"https://chat.test/channels/acme?seq=42", 42, true},
		{"#other:42", 0, false},
		{"https://chat.test/channels/other?seq=42", 0, false},
		{"latest", 0, false},
		{"0", 0, false},
	} {
		got, ok := messageSeq(tc.ref, "acme")
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("messageSeq(%q) = %d, %t; want %d, %t", tc.ref, got, ok, tc.want, tc.ok)
		}
	}
}

func TestANoteListsDecisionsAndOpenQuestionsUnderItsSummary(t *testing.T) {
	body := noteBody(chatsummary.Reply{
		Summary:       "Die Firma bekommt 12% Rabatt, wenn sie bis zum 30.11. unterschreibt und der Vertrag bleibt bei 40 Plätzen.",
		Decisions:     []string{"12% Rabatt"},
		OpenQuestions: []string{"Wer fragt die Rechtsabteilung?"},
	})
	if !strings.HasPrefix(body, "Die Firma bekommt 12% Rabatt") {
		t.Errorf("body = %q, want the summary first", body)
	}
	if !strings.Contains(body, "Entscheidungen:\n- 12% Rabatt") || !strings.Contains(body, "Offene Fragen:\n- Wer fragt") {
		t.Errorf("body = %q, want German headings over each list", body)
	}
	if plain := noteBody(chatsummary.Reply{Summary: "Short."}); plain != "Short." {
		t.Errorf("a summary with no lists = %q, want it alone", plain)
	}
}
