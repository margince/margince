// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailquote

import (
	"testing"
	"unicode/utf8"
)

// ownTextCase is one mail body and the text the rule must keep as the sender's.
type ownTextCase struct {
	name string
	// subject is the mail's own, empty where the case does not turn on it.
	subject string
	body    string
	own     string
}

func runOwnTextCases(t *testing.T, cases []ownTextCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			end := OwnEnd("email", tc.subject, &tc.body)
			if end == nil {
				t.Fatal("a mail body has no stored end")
			}
			if got := string([]rune(tc.body)[:*end]); got != tc.own {
				t.Errorf("own text:\n got %q\nwant %q", got, tc.own)
			}
		})
	}
}

func TestAReplyKeepsTheTextAboveTheQuotedMail(t *testing.T) {
	t.Parallel()
	runOwnTextCases(t, []ownTextCase{
		{
			name: "an English top-posted reply",
			body: "Works for us.\n\nOn Mon, 3 Mar 2025 at 10:15, Dana Roe <dana@buyer.example> wrote:\n> Can you do Friday?\n> Dana\n",
			own:  "Works for us.\n\n",
		},
		{
			name: "a German top-posted reply",
			body: "Passt für uns.\n\nAm 03.03.2025 um 10:15 schrieb Dana Roe <dana@buyer.example>:\n> Geht Freitag?\n",
			own:  "Passt für uns.\n\n",
		},
		{
			name: "a Vietnamese reply that opens with Vào",
			body: "Bên em đồng ý.\n\nVào Th 2, 3 thg 3, 2025 vào lúc 10:15 Nguyễn Văn An <an@buyer.example> đã viết:\n> Thứ Sáu được không?\n",
			own:  "Bên em đồng ý.\n\n",
		},
		{
			name: "an English Outlook block",
			body: "Works for us.\n\nFrom: Dana Roe <dana@buyer.example>\nSent: Monday, March 3, 2025 10:15\nTo: Sales\nSubject: Friday\n\nCan you do Friday?",
			own:  "Works for us.\n\n",
		},
		{
			name: "a German Outlook block under its divider",
			body: "Passt.\n\n-----Ursprüngliche Nachricht-----\nVon: Dana Roe <dana@buyer.example>\nGesendet: Montag, 3. März 2025 10:15\nAn: Vertrieb\nBetreff: Freitag\n\nGeht Freitag?",
			own:  "Passt.\n\n",
		},
		{
			name: "a reply with carriage returns at its line ends",
			body: "Works for us.\r\n\r\nOn Mon, 3 Mar 2025, Sales wrote:\r\n> Friday?\r\n",
			own:  "Works for us.\r\n\r\n",
		},
		{
			name: "the capture preamble stays with the text",
			body: "From: dana@buyer.example\nTo: sales@seller.example\n\nWorks for us.\n\nOn Mon, 3 Mar 2025, Sales wrote:\n> Friday?\n",
			own:  "From: dana@buyer.example\nTo: sales@seller.example\n\nWorks for us.\n\n",
		},
		{
			name: "a signature is the sender still speaking",
			body: "Works for us.\n\n-- \nDana Roe\nBuyer Ltd\n\nOn Mon, 3 Mar 2025, Sales wrote:\n> Friday?\n",
			own:  "Works for us.\n\n-- \nDana Roe\nBuyer Ltd\n\n",
		},
	})
}

func TestTheRuleNeverCutsTheSendersWords(t *testing.T) {
	t.Parallel()
	inline := "On Mon, 3 Mar 2025, Sales wrote:\n> Can you do Friday?\nFriday works.\n> And the price?\nAs quoted.\n"
	bottom := "On Mon, 3 Mar 2025, Sales wrote:\n> Can you do Friday?\n\nFriday works.\n"
	onlyQuote := "> Can you do Friday?\n> Sales\n"
	prose := "Von: der Kollegin habe ich nichts gehört.\nOn the other hand, nobody wrote: anything."
	runOwnTextCases(t, []ownTextCase{
		{name: "an inline reply keeps every answer", body: inline, own: inline},
		{name: "a bottom-posted reply keeps the answer under the quote", body: bottom, own: bottom},
		{name: "a body that is only a quote keeps it", body: onlyQuote, own: onlyQuote},
		{name: "prose that names a sender is not a boundary", body: prose, own: prose},
		{
			name: "an inline reply ends at the trailing quote",
			body: "> Can you do Friday?\nFriday works.\n\n> Older mail\n> Sales\n",
			own:  "> Can you do Friday?\nFriday works.\n\n",
		},
	})
}

// A forward line is read as the shared patterns read it: it ends the sender's
// own text, as it does where the History tab folds.
func TestAForwardLineEndsTheSendersOwnText(t *testing.T) {
	t.Parallel()
	runOwnTextCases(t, []ownTextCase{
		{
			name: "an English forward",
			body: "FYI, the buyer's terms.\n\n---------- Forwarded message ---------\nFrom: Dana Roe <dana@buyer.example>\nDate: Mon, 3 Mar 2025 at 10:15\n\nNet thirty days.\n",
			own:  "FYI, the buyer's terms.\n\n",
		},
		{
			name: "a German forward",
			body: "Zur Info.\n\n-------- Weitergeleitete Nachricht --------\nVon: Dana Roe <dana@buyer.example>\n\nDreißig Tage netto.\n",
			own:  "Zur Info.\n\n",
		},
	})
}

// Search finds a forward by the mail it passes on, and a reply with the same
// block only by its own words.
func TestAForwardKeepsTheMailItPassesOn(t *testing.T) {
	t.Parallel()
	outlook := "Zur Info.\n\nVon: Max Muster <max@kunde.de>\nGesendet: Dienstag, 12. August 2026 09:14\nAn: Lars\nBetreff: Angebot\n\nPasst Dienstag?"
	runOwnTextCases(t, []ownTextCase{
		{name: "a German Outlook forward with no forward line", subject: "WG: Angebot", body: outlook, own: outlook},
		{name: "the same block under a reply subject", subject: "AW: Angebot", body: outlook, own: "Zur Info.\n\n"},
		{
			name:    "an English Outlook forward stops at the history it quotes",
			subject: "FW: Pricing",
			body:    "FYI\n\nFrom: Dana Roe\nSent: Monday\nTo: Sales\n\nThirty days net.\n\nFrom: Sales\nSent: Friday\nTo: Dana Roe\n\nTerms?",
			own:     "FYI\n\nFrom: Dana Roe\nSent: Monday\nTo: Sales\n\nThirty days net.\n\n",
		},
		{
			name:    "a Gmail forward keeps the block under its forward line",
			subject: "Fwd: Pricing",
			body:    "FYI\n\n---------- Forwarded message ---------\nFrom: Dana Roe <dana@buyer.example>\nDate: Mon, 3 Mar 2025\nTo: Sales\n\nThirty days net.",
			own:     "FYI\n\n---------- Forwarded message ---------\nFrom: Dana Roe <dana@buyer.example>\nDate: Mon, 3 Mar 2025\nTo: Sales\n\nThirty days net.",
		},
		{
			name:    "a reply to a forward is a reply",
			subject: "Re: Fwd: Pricing",
			body:    "Agreed.\n\nFrom: Dana Roe\nSent: Monday\nTo: Sales\n\nThirty days net.",
			own:     "Agreed.\n\n",
		},
		{
			name:    "a marked quote right under the forwarded block ends the text",
			subject: "FW: Pricing",
			body:    "FYI\n\nFrom: Dana Roe\nSent: Monday\nTo: Sales\nThirty days net.\n> Terms?",
			own:     "FYI\n\nFrom: Dana Roe\nSent: Monday\nTo: Sales\nThirty days net.\n",
		},
		{
			name:    "a marked quote in a forward still ends the text",
			subject: "Fwd: Pricing",
			body:    "Agreed.\n\n> Thirty days net?\n",
			own:     "Agreed.\n\n",
		},
	})
}

func TestOnlyAMailBodyHasAStoredEnd(t *testing.T) {
	t.Parallel()
	body := "Works for us.\n\n> Friday?\n"
	if end := OwnEnd("note", "", &body); end != nil {
		t.Errorf("a note stores the end %d; a member wrote every line of it", *end)
	}
	if end := OwnEnd("email", "", nil); end != nil {
		t.Errorf("a mail with no body stores the end %d", *end)
	}
	empty := ""
	if end := OwnEnd("email", "", &empty); end == nil || *end != 0 {
		t.Errorf("an empty mail body stores %v, want 0", end)
	}
}

// The database cuts with left(body, n), which counts characters. A byte count
// would cut a Vietnamese or German reply short.
func TestTheStoredEndCountsCharacters(t *testing.T) {
	t.Parallel()
	own := "Bên em đồng ý với điều khoản.\n"
	body := own + "> Thứ Sáu được không?\n"
	end := OwnEnd("email", "", &body)
	if end == nil || *end != utf8.RuneCountInString(own) {
		t.Fatalf("stored end %v, want %d characters", end, utf8.RuneCountInString(own))
	}
	if *end >= len(own) {
		t.Fatalf("the fixture has no multi-byte character before the cut: %d characters, %d bytes", *end, len(own))
	}
}
