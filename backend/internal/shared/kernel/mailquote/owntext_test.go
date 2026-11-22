// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailquote

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// ownTextCase is one mail body and the text the rule must keep as the sender's.
type ownTextCase struct {
	name string
	body string
	own  string
}

func runOwnTextCases(t *testing.T, cases []ownTextCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			end := OwnEnd("email", &tc.body)
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
			name: "a Vietnamese reply that opens with Ngày",
			body: "Bên em đồng ý.\n\nNgày 3 thg 3, 2025, lúc 10:15, Nguyễn Văn An <an@buyer.example> đã viết:\n\nThứ Sáu được không?\n",
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
			name: "a Vietnamese Outlook block",
			body: "Đồng ý.\n\nTừ: Nguyễn Văn An <an@buyer.example>\nĐã gửi: Thứ Hai, 3 Tháng 3, 2025 10:15\nTới: Kinh doanh\nChủ đề: Thứ Sáu\n\nThứ Sáu được không?",
			own:  "Đồng ý.\n\n",
		},
		{
			name: "a Vietnamese original-message divider",
			body: "Đồng ý.\n\n-----Thư gốc-----\nThứ Sáu được không?",
			own:  "Đồng ý.\n\n",
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

func TestAForwardKeepsTheMailItPassesOn(t *testing.T) {
	t.Parallel()
	gmail := "FYI, the buyer's terms.\n\n---------- Forwarded message ---------\nFrom: Dana Roe <dana@buyer.example>\nDate: Mon, 3 Mar 2025 at 10:15\nSubject: Terms\nTo: Sales <sales@seller.example>\n\n\nNet thirty days.\n"
	apple := "Begin forwarded message:\n\nFrom: Dana Roe <dana@buyer.example>\nSubject: Terms\nDate: 3 March 2025 at 10:15:00 CET\nTo: Sales <sales@seller.example>\n\nNet thirty days.\n"
	german := "Zur Info.\n\n-------- Weitergeleitete Nachricht --------\nVon: Dana Roe <dana@buyer.example>\nDatum: Mo., 3. März 2025 10:15\nBetreff: Konditionen\nAn: Vertrieb\n\nDreißig Tage netto.\n"
	germanApple := "Anfang der weitergeleiteten Nachricht:\n\nVon: Dana Roe <dana@buyer.example>\nBetreff: Konditionen\nDatum: 3. März 2025 um 10:15:00 MEZ\nAn: Vertrieb\n\nDreißig Tage netto.\n"
	vietnamese := "Gửi anh tham khảo.\n\n---------- Thư đã chuyển tiếp ---------\nTừ: Nguyễn Văn An <an@buyer.example>\nNgày: Th 2, 3 thg 3, 2025 vào lúc 10:15\nTiêu đề: Điều khoản\nĐến: Kinh doanh\n\nThanh toán trong ba mươi ngày.\n"
	marked := "Passt, machen wir so.\n\n---------- Forwarded message ----------\n> Wollen wir Dienstag sprechen?"
	runOwnTextCases(t, []ownTextCase{
		{name: "a forward marked on every line", body: marked, own: marked},
		{name: "an English forward", body: gmail, own: gmail},
		{name: "an English forward with no words above it", body: apple, own: apple},
		{name: "a German forward", body: german, own: german},
		{name: "a German forward with no words above it", body: germanApple, own: germanApple},
		{name: "a Vietnamese forward", body: vietnamese, own: vietnamese},
		{
			name: "the history under a forwarded mail is still a quote",
			body: gmail + "\nOn Fri, 28 Feb 2025, Sales wrote:\n> What are your terms?\n",
			own:  gmail + "\n",
		},
	})
}

func TestOnlyAMailBodyHasAStoredEnd(t *testing.T) {
	t.Parallel()
	body := "Works for us.\n\n> Friday?\n"
	if end := OwnEnd("note", &body); end != nil {
		t.Errorf("a note stores the end %d; a member wrote every line of it", *end)
	}
	if end := OwnEnd("email", nil); end != nil {
		t.Errorf("a mail with no body stores the end %d", *end)
	}
	empty := ""
	if end := OwnEnd("email", &empty); end == nil || *end != 0 {
		t.Errorf("an empty mail body stores %v, want 0", end)
	}
}

// The database cuts with left(body, n), which counts characters. A byte count
// would cut a Vietnamese or German reply short.
func TestTheStoredEndCountsCharacters(t *testing.T) {
	t.Parallel()
	own := "Bên em đồng ý với điều khoản.\n"
	body := own + "> Thứ Sáu được không?\n"
	end := OwnEnd("email", &body)
	if end == nil || *end != utf8.RuneCountInString(own) {
		t.Fatalf("stored end %v, want %d characters", end, utf8.RuneCountInString(own))
	}
	if *end >= len(own) {
		t.Fatalf("the fixture has no multi-byte character before the cut: %d characters, %d bytes", *end, len(own))
	}
}

func TestAForwardLineIsNotAReplyLine(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		"---------- Forwarded message ---------", "-------- Weitergeleitete Nachricht --------",
		"---------- Thư đã chuyển tiếp ---------", "Begin forwarded message:",
		"Anfang der weitergeleiteten E-Mail:", "Thư được chuyển tiếp:",
	} {
		if !isForward(line) || isReply(line) || !IsAttribution(line) {
			t.Errorf("%q: forward %t, reply %t, attribution %t; want a forward only",
				line, isForward(line), isReply(line), IsAttribution(line))
		}
	}
	for _, line := range []string{
		"-----Original Message-----", "-----Ursprüngliche Nachricht-----", "-----Thư gốc-----",
		"On Mon, 3 Mar 2025, Sales wrote:", "Ngày 3 thg 3, 2025, An đã viết:", strings.Repeat("_", 32),
	} {
		if isForward(line) || !isReply(line) {
			t.Errorf("%q: forward %t, reply %t; want a reply only", line, isForward(line), isReply(line))
		}
	}
}
