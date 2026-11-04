// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textlang

import (
	"strings"
	"testing"
)

// A voice learns from what the user typed in a sent mail. A quoted reply, a
// forward, a signature or a legal notice was written by somebody else or by a
// template. It would teach the voice their words.
func TestOwnWordsKeepsOnlyWhatTheSenderTyped(t *testing.T) {
	t.Parallel()
	longGermanBody := strings.Repeat("Ich schicke dir morgen den Entwurf. ", 40)
	for name, tc := range map[string]struct{ body, want string }{
		"plain mail is unchanged": {
			body: "Hi Anna,\n\nthe offer is attached. Call me if anything is unclear.\n\nLars",
			want: "Hi Anna,\n\nthe offer is attached. Call me if anything is unclear.\n\nLars",
		},
		"the stored envelope block is dropped": {
			body: "From: lars@gradion.com\nTo: anna@kunde.de\n\nPasst, bis Freitag.",
			want: "Passt, bis Freitag.",
		},
		"an English reply drops the quoted thread": {
			body: "Works for me.\n\nOn Mon, 3 Nov 2026 at 10:00, Anna Berg <anna@kunde.de> wrote:\n> Can we meet Friday?",
			want: "Works for me.",
		},
		"a German reply drops the quoted thread": {
			body: "Klar, gerne.\n\nAm 03.11.2026 um 10:00 schrieb Anna Berg <anna@kunde.de>:\n> Passt dir Freitag?",
			want: "Klar, gerne.",
		},
		"an Outlook original-message block is cut": {
			body: "See below.\n\n-----Original Message-----\nFrom: anna@kunde.de\n\nThe question.",
			want: "See below.",
		},
		"a Vietnamese forward header is cut": {
			body: "Gửi anh xem.\n\n---------- Thư được chuyển tiếp ----------\nTừ: anna@kunde.vn\n\nNội dung gốc.",
			want: "Gửi anh xem.",
		},
		"a Vietnamese reply drops the quoted thread": {
			body: "Dạ, em cảm ơn anh.\n\nVào Th 2, 3 thg 11, 2026 vào lúc 10:00 Anna <anna@kunde.vn> đã viết:\nAnh có rảnh thứ Sáu không?",
			want: "Dạ, em cảm ơn anh.",
		},
		"an Outlook header block in the middle of the body is cut": {
			body: "Passt, danke.\n\nVon: Anna Berg <anna@kunde.de>\nGesendet: Montag, 3. November 2026 10:00\nAn: Lars <lars@gradion.com>\nBetreff: Termin\n\nPasst dir Freitag?",
			want: "Passt, danke.",
		},
		"a '>' quote is cut": {
			body: "Agreed.\n> earlier text from Anna",
			want: "Agreed.",
		},
		"the sig-dash signature is cut": {
			body: "Thanks, see you Friday.\n-- \nLars Jankowfsky\nGradion GmbH",
			want: "Thanks, see you Friday.",
		},
		"a legal footer is cut": {
			body: "Here is the draft.\n\nThis email and any attachments are confidential and intended solely for the addressee named above.",
			want: "Here is the draft.",
		},
		"a reply that is only a quote has no own words": {
			body: "> Can we meet Friday?\n> Anna",
			want: "",
		},
		"a familiar German mail keeps its greeting and length": {
			body: "Hallo Jan,\n\nkurz und knapp: passt dir Dienstag?\n\nLG Lars",
			want: "Hallo Jan,\n\nkurz und knapp: passt dir Dienstag?\n\nLG Lars",
		},
		"a long mail is not bounded": {
			body: longGermanBody,
			want: strings.TrimSpace(longGermanBody),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := OwnWords(tc.body); got != tc.want {
				t.Errorf("OwnWords(%q)\n got: %q\nwant: %q", tc.body, got, tc.want)
			}
		})
	}
}

// MessageOpening is the bounded reading of the same text, so the two never
// disagree on where the sender's words end.
func TestMessageOpeningIsTheStartOfOwnWords(t *testing.T) {
	t.Parallel()
	body := "From: lars@gradion.com\n\nKlar, gerne. Ich melde mich morgen.\n\nAm 03.11.2026 schrieb Anna <a@k.de>:\n> Frage"
	own := OwnWords(body)
	if got := MessageOpening(body, 12); !strings.HasPrefix(own, got) || got == "" {
		t.Errorf("MessageOpening = %q, not a prefix of OwnWords %q", got, own)
	}
}
