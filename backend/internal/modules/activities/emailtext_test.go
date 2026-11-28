// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/mailquote"
)

// The cases here are the shared fixtures the frontend splitter is held to as
// well. A case added on one side and not the other is the drift this file and
// gates/frontendemailtext_test.go exist to catch, so the table is written to be
// read from both languages: one body in, one main, one trimmed and one tail out.
//
// That gate did not exist when this sentence was first written. It was added
// with the tail field, and it found the two tables sharing almost no bodies at
// all — thirteen here the browser never ran, ten there this file never ran. The
// splitters agreed on every one of them, so nothing was broken; but nothing was
// held either, and the comment said otherwise. A claimed protection nobody
// wrote is worse than silence, because the next author greps, finds the
// sentence, and stops looking.

// splitCase is one body and what the splitter must make of it. The three tables
// below share it, and the shared runner asserts all three fields for every one
// of them.
type splitCase struct {
	name string
	// subject is the mail's own, empty where the case does not turn on it.
	subject string
	body    string
	main    string
	trimmed string
	// tail is required wherever trimmed is non-empty; a case with nothing
	// trimmed is checked against TailNone without stating it.
	tail EmailBodyTail
}

// runSplitCases asserts what the splitter made of each body.
//
// One runner for the three tables, so a case moved between them keeps being
// checked the same way — three copies of these assertions would be three
// chances for one table to stop asserting the tail.
func runSplitCases(t *testing.T, cases []splitCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SplitEmailBody(tc.body, tc.subject)
			if got.Main != tc.main {
				t.Errorf("main:\n got %q\nwant %q", got.Main, tc.main)
			}
			if got.Trimmed != tc.trimmed {
				t.Errorf("trimmed:\n got %q\nwant %q", got.Trimmed, tc.trimmed)
			}
			// What KIND of tail, stated by every case that has one. A signature
			// and a quote are trimmed for different reasons, and the viewer
			// labels the control it folds them behind from this — it put a
			// sender's own sign-off behind "show quoted history" on every
			// message that had no history under it.
			//
			// A case with nothing trimmed says so here too, so "no tail" is
			// asserted rather than left as the zero value nobody looked at.
			want := tc.tail
			if tc.trimmed == "" {
				want = TailNone
			}
			if got.Tail != want {
				t.Errorf("tail: got %q, want %q", got.Tail, want)
			}
		})
	}
}

// A body the splitter must NOT cut: the whole thing is what the sender wrote.
//
// These are the cases a keen splitter gets wrong. A "Von:" line that is prose,
// mobile boilerplate matching mid-sentence, a body that is only a quote or only
// a sign-off — each looks like a boundary and is not one, and cutting any of
// them hides the message behind a fold.
func TestABodyWithNoTailIsAllMessage(t *testing.T) {
	t.Parallel()
	runSplitCases(t, []splitCase{
		{
			name: "a plain body is all message",
			body: "Können wir Dienstag sprechen?",
			main: "Können wir Dienstag sprechen?",
		},
		{
			name: "a Von: line without a sent-date is prose",
			body: "Von: uns beiden kam bisher keine Antwort.",
			main: "Von: uns beiden kam bisher keine Antwort.",
		},
		{
			name: "mobile boilerplate matches the whole line, not a prefix",
			body: "Sent from my perspective the contract is not ready",
			main: "Sent from my perspective the contract is not ready",
		},
		{
			name: "a greeting alone is still a message",
			body: "Danke!",
			main: "Danke!",
		},
		{
			name: "a body that is only a quote keeps the quote as its text",
			body: "> Passt Dienstag?",
			main: "> Passt Dienstag?",
		},
		{
			name: "the capture preamble is peeled and does not fold the message",
			body: "From: ana@example.test\nTo: lars@example.test\n\nPasst Dienstag?",
			main: "Passt Dienstag?",
		},
		{
			name: "an empty body stays empty",
			body: "   \n\n ",
		},
		// Below: the cases the BROWSER's table already held and this one did
		// not. The header above promised a gate holding the two tables to one
		// set of bodies; the gate did not exist, and when it was written it
		// found the two sides testing largely different rules. Every case here
		// was run against this splitter before being written down — the two
		// agree on all of them, so this closes a testing gap rather than a
		// behavioural one.
		{
			name: "a quoted reply with no message above it is its own text",
			body: "> Passt Dienstag?\n> Anna",
			main: "> Passt Dienstag?\n> Anna",
		},
		{
			name: "a preamble over nothing but a signature keeps the signature as the message",
			body: "From: anna@kunde.de\nTo: lars@gradion.com\n\n-- \nAnna Berger",
			main: "-- \nAnna Berger",
		},
		{
			name: "a body that is only a sign-off is still the message",
			body: "Viele Grüße\nAnna",
			main: "Viele Grüße\nAnna",
		},
		{
			name: "a preamble over a plain question peels to the question",
			body: "From: anna@kunde.de\nTo: lars@gradion.com\n\nKönnen wir Dienstag sprechen?",
			main: "Können wir Dienstag sprechen?",
		},
		{
			name: "whitespace with no newline pair is still empty",
			body: "   \n  ",
		},
		{
			name: "a plain statement is all message",
			body: "Ja, Dienstag passt.",
			main: "Ja, Dienstag passt.",
		},
		{
			name: "mobile wording mid-sentence is prose, not a footer",
			body: "Kurzes Update:\nSent from my perspective, the contract is not ready.\nBitte noch nicht rausschicken.",
			main: "Kurzes Update:\nSent from my perspective, the contract is not ready.\nBitte noch nicht rausschicken.",
		},
		{
			name: "a preamble with nothing under it is the message",
			body: "From: a@example.com\nTo: b@example.com\n\n",
			main: "From: a@example.com\nTo: b@example.com",
		},
		{
			name: "a short sign-off form opening a sentence is prose",
			body: "LG Waschmaschinen sind auch im Angebot, die Lieferung dauert aber acht Wochen. Best of luck damit.",
			main: "LG Waschmaschinen sind auch im Angebot, die Lieferung dauert aber acht Wochen. Best of luck damit.",
		},
		{
			name: "a Von: sentence spanning two lines is prose",
			body: "Von: der Messe habe ich drei Kontakte mitgebracht.\nAlle drei wollen ein Angebot.",
			main: "Von: der Messe habe ich drei Kontakte mitgebracht.\nAlle drei wollen ein Angebot.",
		},
	})
}

// A sign-off: the sender still speaking, trimmed from the message and labelled
// TailSignature so the viewer shows it rather than folding it away.
//
// The KIND is the point. One label for both tails put a sender's own name
// behind "show quoted history" on messages that had no history at all.
func TestASignOffIsTrimmedAndNamedAsOne(t *testing.T) {
	t.Parallel()
	runSplitCases(t, []splitCase{
		{
			name:    "the RFC 3676 delimiter opens the signature",
			body:    "Passt bei mir.\n\n--\nAna Sommer\nGeschäftsführerin",
			main:    "Passt bei mir.",
			trimmed: "--\nAna Sommer\nGeschäftsführerin",
			tail:    TailSignature,
		},
		{
			name:    "a German sign-off near the end closes the message",
			body:    "Danke für das Angebot.\n\nViele Grüße\nAna",
			main:    "Danke für das Angebot.",
			trimmed: "Viele Grüße\nAna",
			tail:    TailSignature,
		},
		{
			name:    "and folds when it is the whole line",
			body:    "Passt.\n\nSent from my iPhone",
			main:    "Passt.",
			trimmed: "Sent from my iPhone",
			tail:    TailSignature,
		},
		{
			name:    "a delimiter signature block folds whole",
			body:    "Anbei das Angebot.\n\n-- \nAnna Berger\nKunde GmbH\n+49 89 123456",
			main:    "Anbei das Angebot.",
			trimmed: "-- \nAnna Berger\nKunde GmbH\n+49 89 123456",
			tail:    TailSignature,
		},
		{
			name:    "a formal German sign-off takes its title block with it",
			body:    "Danke für das Gespräch. Ich melde mich nächste Woche.\n\nMit freundlichen Grüßen\nAnna Berger\nGeschäftsführerin\nKunde GmbH",
			main:    "Danke für das Gespräch. Ich melde mich nächste Woche.",
			trimmed: "Mit freundlichen Grüßen\nAnna Berger\nGeschäftsführerin\nKunde GmbH",
			tail:    TailSignature,
		},
		{
			name:    "a bare sign-off with a quote under it opens a signature, not a quote",
			body:    "Kurz zur Rückfrage: ja.\n\nViele Grüße\nAnna\n\nAm 12.08.2026 schrieb Max:\n> Passt Dienstag?",
			main:    "Kurz zur Rückfrage: ja.",
			trimmed: "Viele Grüße\nAnna\n\nAm 12.08.2026 schrieb Max:\n> Passt Dienstag?",
			tail:    TailSignature,
		},
		// The message that shipped read wrong: a sign-off, and no quoted
		// history anywhere under it. The tail's KIND is the whole point of the
		// case — the viewer folded this sender's own name behind "show quoted
		// history" on a message that had none.
		{
			name:    "a sign-off with no quote under it opens a signature",
			body:    "Hallo zusammen,\n\nanbei die Zusammenfassung von gestern.\n\nIch schicke bis Ende der Woche eine Aufwandsschätzung.\n\nViele Grüße\nBảo",
			main:    "Hallo zusammen,\n\nanbei die Zusammenfassung von gestern.\n\nIch schicke bis Ende der Woche eine Aufwandsschätzung.",
			trimmed: "Viele Grüße\nBảo",
			tail:    TailSignature,
		},
	})
}

// An older message under this one, trimmed and labelled TailQuote — the tail
// the viewer folds.
//
// An attribution line travels WITH the quote it introduces, and an Outlook
// header block travels as one: a quote cut below its own "Am 1. September
// schrieb Ana:" reads as the reply having said it.
func TestQuotedHistoryIsTrimmedAndNamedAsOne(t *testing.T) {
	t.Parallel()
	runSplitCases(t, []splitCase{
		{
			name:    "an attribution line travels with the quote it introduces",
			body:    "Ja, gerne.\n\nAm 1. September schrieb Ana:\n> Passt Dienstag?",
			main:    "Ja, gerne.",
			trimmed: "Am 1. September schrieb Ana:\n> Passt Dienstag?",
			tail:    TailQuote,
		},
		{
			name:    "the Outlook block needs its sent-date neighbour",
			body:    "Siehe unten.\n\nVon: Ana Sommer\nGesendet: Montag, 1. September\nAn: Lars\n\nPasst Dienstag?",
			main:    "Siehe unten.",
			trimmed: "Von: Ana Sommer\nGesendet: Montag, 1. September\nAn: Lars\n\nPasst Dienstag?",
			tail:    TailQuote,
		},
		{
			name:    "a quote marker with no attribution above it opens a quote",
			body:    "Ja, passt.\n\n> Passt Dienstag?\n> Anna",
			main:    "Ja, passt.",
			trimmed: "> Passt Dienstag?\n> Anna",
			tail:    TailQuote,
		},
		{
			name:    "a full Outlook header block travels as one quote",
			body:    "Sehe ich auch so.\n\nVon: Max Muster <max@kunde.de>\nGesendet: Dienstag, 12. August 2026 09:14\nAn: Lars\nBetreff: AW: Angebot\n\nPasst Dienstag?",
			main:    "Sehe ich auch so.",
			trimmed: "Von: Max Muster <max@kunde.de>\nGesendet: Dienstag, 12. August 2026 09:14\nAn: Lars\nBetreff: AW: Angebot\n\nPasst Dienstag?",
			tail:    TailQuote,
		},
		{
			// The numeric date form, beside the written-out one above: both
			// spellings reach the same attribution rule, and a table holding
			// only one would not notice a pattern that stopped matching the
			// other.
			name:    "a numeric-date attribution also travels with its quote",
			body:    "Ja, gerne.\n\nAm 12.08.2026 schrieb Max:\n> Passt Dienstag?",
			main:    "Ja, gerne.",
			trimmed: "Am 12.08.2026 schrieb Max:\n> Passt Dienstag?",
			tail:    TailQuote,
		},
	})
}

// A forward's first header block opens the forwarded mail, which stays in the
// message. The same block under a reply's subject is older mail and folds.
func TestAForwardKeepsTheMailItForwards(t *testing.T) {
	t.Parallel()
	runSplitCases(t, []splitCase{
		{
			name:    "a German Outlook forward with no forward line keeps the forwarded mail",
			subject: "WG: Angebot",
			body:    "Zur Info.\n\nVon: Max Muster <max@kunde.de>\nGesendet: Dienstag, 12. August 2026 09:14\nAn: Lars\nBetreff: Angebot\n\nPasst Dienstag?",
			main:    "Zur Info.\n\nVon: Max Muster <max@kunde.de>\nGesendet: Dienstag, 12. August 2026 09:14\nAn: Lars\nBetreff: Angebot\n\nPasst Dienstag?",
		},
		{
			name:    "the same block under a reply subject folds",
			subject: "AW: Angebot",
			body:    "Zur Info.\n\nVon: Max Muster <max@kunde.de>\nGesendet: Dienstag, 12. August 2026 09:14\nAn: Lars\nBetreff: Angebot\n\nPasst Dienstag?",
			main:    "Zur Info.",
			trimmed: "Von: Max Muster <max@kunde.de>\nGesendet: Dienstag, 12. August 2026 09:14\nAn: Lars\nBetreff: Angebot\n\nPasst Dienstag?",
			tail:    TailQuote,
		},
		{
			name:    "a reply to a forward is a reply",
			subject: "Re: Fwd: Angebot",
			body:    "Gut, dann so.\n\nFrom: Max Muster\nSent: Tuesday, 12 August 2026\nTo: Lars\n\nPasst Dienstag?",
			main:    "Gut, dann so.",
			trimmed: "From: Max Muster\nSent: Tuesday, 12 August 2026\nTo: Lars\n\nPasst Dienstag?",
			tail:    TailQuote,
		},
		{
			name:    "a Gmail forward line and its sender block stay in the message",
			subject: "Fwd: Angebot",
			body:    "Siehe unten.\n\n---------- Forwarded message ---------\nFrom: Max Muster <max@kunde.de>\nDate: Tue, 12 Aug 2026 09:14\nSubject: Angebot\nTo: Lars\n\nPasst Dienstag?",
			main:    "Siehe unten.\n\n---------- Forwarded message ---------\nFrom: Max Muster <max@kunde.de>\nDate: Tue, 12 Aug 2026 09:14\nSubject: Angebot\nTo: Lars\n\nPasst Dienstag?",
		},
		{
			name:    "a Vietnamese Gmail forward keeps its sender block",
			subject: "Fwd: Báo giá",
			body:    "Gửi anh xem.\n\n---------- Thư được chuyển tiếp ---------\nTừ: Nguyễn Văn An <an@kunde.vn>\nNgày: Th 3, 12 thg 8, 2026\nTiêu đề: Báo giá\nĐến: Lars\n\nThứ Ba được không?",
			main:    "Gửi anh xem.\n\n---------- Thư được chuyển tiếp ---------\nTừ: Nguyễn Văn An <an@kunde.vn>\nNgày: Th 3, 12 thg 8, 2026\nTiêu đề: Báo giá\nĐến: Lars\n\nThứ Ba được không?",
		},
		{
			name:    "a Vietnamese Outlook block under a reply subject folds",
			subject: "Re: Báo giá",
			body:    "Đồng ý.\n\nTừ: Nguyễn Văn An <an@kunde.vn>\nĐã gửi: Thứ Ba, 12 tháng 8, 2026 09:14\nĐến: Lars\nChủ đề: Báo giá\n\nThứ Ba được không?",
			main:    "Đồng ý.",
			trimmed: "Từ: Nguyễn Văn An <an@kunde.vn>\nĐã gửi: Thứ Ba, 12 tháng 8, 2026 09:14\nĐến: Lars\nChủ đề: Báo giá\n\nThứ Ba được không?",
			tail:    TailQuote,
		},
		{
			name:    "an Apple Mail forward keeps the block under its blank line",
			subject: "Fwd: Angebot",
			body:    "FYI\n\nBegin forwarded message:\n\nFrom: Max Muster <max@kunde.de>\nSubject: Angebot\nDate: 12 August 2026\nTo: Lars\n\nPasst Dienstag?",
			main:    "FYI\n\nBegin forwarded message:\n\nFrom: Max Muster <max@kunde.de>\nSubject: Angebot\nDate: 12 August 2026\nTo: Lars\n\nPasst Dienstag?",
		},
		{
			name:    "an Outlook divider over the forwarded block stays with it",
			subject: "WG: Angebot",
			body:    "Zur Info.\n\n________________________________\nVon: Max Muster\nGesendet: Dienstag\nAn: Lars\n\nPasst Dienstag?",
			main:    "Zur Info.\n\n________________________________\nVon: Max Muster\nGesendet: Dienstag\nAn: Lars\n\nPasst Dienstag?",
		},
		{
			name:    "the history the forwarded mail quotes still folds",
			subject: "WG: Angebot",
			body:    "Zur Info.\n\nVon: Max Muster\nGesendet: Dienstag\nAn: Lars\n\nPasst Dienstag?\n\nVon: Lars\nGesendet: Montag\nAn: Max Muster\n\nWann passt es?",
			main:    "Zur Info.\n\nVon: Max Muster\nGesendet: Dienstag\nAn: Lars\n\nPasst Dienstag?",
			trimmed: "Von: Lars\nGesendet: Montag\nAn: Max Muster\n\nWann passt es?",
			tail:    TailQuote,
		},
		{
			name:    "a forward with no note of its own opens on the forwarded block",
			subject: "FW: Angebot",
			body:    "Von: Max Muster\nGesendet: Dienstag\nAn: Lars\n\nPasst Dienstag?\n\nVon: Lars\nGesendet: Montag\nAn: Max Muster\n\nWann passt es?",
			main:    "Von: Max Muster\nGesendet: Dienstag\nAn: Lars\n\nPasst Dienstag?",
			trimmed: "Von: Lars\nGesendet: Montag\nAn: Max Muster\n\nWann passt es?",
			tail:    TailQuote,
		},
		{
			name:    "a quote right under the forwarded block still folds",
			subject: "FW: Pricing",
			body:    "FYI\n\nFrom: Dana\nSent: Monday\nTo: Sales\nTerms.\n> Earlier message",
			main:    "FYI\n\nFrom: Dana\nSent: Monday\nTo: Sales\nTerms.",
			trimmed: "> Earlier message",
			tail:    TailQuote,
		},
		{
			name:    "an inline reply in the note does not hide the forwarded mail",
			subject: "FW: Pricing",
			body:    "Regarding this:\n> Please send the terms\nHere they are.\n\nFrom: Dana\nSent: Monday\nTo: Sales\n\nNet thirty.",
			main:    "Regarding this:\n> Please send the terms\nHere they are.\n\nFrom: Dana\nSent: Monday\nTo: Sales\n\nNet thirty.",
		},
		{
			name:    "a second header right under the forwarded block opens the history",
			subject: "FW: Pricing",
			body:    "FYI\n\nFrom: Dana\nSent: Monday\nTo: Sales\nTerms.\nFrom: Sales\nSent: Friday\nTo: Dana\nOld terms.",
			main:    "FYI\n\nFrom: Dana\nSent: Monday\nTo: Sales\nTerms.",
			trimmed: "From: Sales\nSent: Friday\nTo: Dana\nOld terms.",
			tail:    TailQuote,
		},
		{
			name:    "a sign-off above the forwarded block closes only the note",
			subject: "FW: Pricing",
			body:    "FYI\n\nBest regards\nAlice\n\nFrom: Dana\nSent: Monday\nTo: Sales\n\nTerms.",
			main:    "FYI\n\nBest regards\nAlice\n\nFrom: Dana\nSent: Monday\nTo: Sales\n\nTerms.",
		},
		{
			name:    "a quote marker in a forward still folds",
			subject: "Fwd: Angebot",
			body:    "Ja, passt.\n\n> Passt Dienstag?",
			main:    "Ja, passt.",
			trimmed: "> Passt Dienstag?",
			tail:    TailQuote,
		},
	})
}

// The boundary the search value is cut at. An inline answer stays in the
// message; quoted mail folds only when nothing follows it.
func TestOnlyQuotedEarlierMailLeavesTheMessage(t *testing.T) {
	t.Parallel()
	runSplitCases(t, []splitCase{
		{
			name: "an inline reply keeps the answers under each quote",
			body: "On Mon, 3 Mar 2025, Sales wrote:\n> Can you do Friday?\nFriday works.\n> And the price?\nAs quoted.",
			main: "On Mon, 3 Mar 2025, Sales wrote:\n> Can you do Friday?\nFriday works.\n> And the price?\nAs quoted.",
		},
		{
			name:    "a signature above the quoted mail still opens the tail",
			body:    "Works for us.\n\n-- \nDana Roe\n\n> Friday?",
			main:    "Works for us.",
			trimmed: "-- \nDana Roe\n\n> Friday?",
			tail:    TailSignature,
		},
	})
}

func TestEmailSummaryTextIsOneLineOfTheSendersOwnWords(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "newlines collapse to single spaces",
			body: "Passt Dienstag?\n\nOder Mittwoch?\n\nViele Grüße\nAna",
			want: "Passt Dienstag? Oder Mittwoch?",
		},
		{
			name: "a body that is only a forward previews the forward",
			body: "> Passt Dienstag?",
			want: "> Passt Dienstag?",
		},
		{
			name: "an empty body previews nothing",
			body: "",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := EmailSummaryText(tc.body, ""); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// mailquote cannot import the contract, so it spells the mail kind itself.
func TestTheSplitterAndTheContractNameMailTheSame(t *testing.T) {
	t.Parallel()
	if mailquote.MailKind != string(crmcontracts.ActivityKindEmail) {
		t.Errorf("mailquote calls mail %q and the contract calls it %q", mailquote.MailKind, crmcontracts.ActivityKindEmail)
	}
}
