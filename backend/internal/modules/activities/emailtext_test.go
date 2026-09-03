// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

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

func TestSplitEmailBodyFindsWhatTheSenderWrote(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		body    string
		main    string
		trimmed string
		// tail is required wherever trimmed is non-empty; a case with nothing
		// trimmed is checked against TailNone without stating it.
		tail EmailBodyTail
	}{
		{
			name: "a plain body is all message",
			body: "Können wir Dienstag sprechen?",
			main: "Können wir Dienstag sprechen?",
		},
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
			name:    "and folds when it is the whole line",
			body:    "Passt.\n\nSent from my iPhone",
			main:    "Passt.",
			trimmed: "Sent from my iPhone",
			tail:    TailSignature,
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
			name: "a preamble over nothing but a signature keeps the signature as the message",
			body: "From: anna@kunde.de\nTo: lars@gradion.com\n\n-- \nAnna Berger",
			main: "-- \nAnna Berger",
		},
		{
			name:    "a bare sign-off with a quote under it opens a signature, not a quote",
			body:    "Kurz zur Rückfrage: ja.\n\nViele Grüße\nAnna\n\nAm 12.08.2026 schrieb Max:\n> Passt Dienstag?",
			main:    "Kurz zur Rückfrage: ja.",
			trimmed: "Viele Grüße\nAnna\n\nAm 12.08.2026 schrieb Max:\n> Passt Dienstag?",
			tail:    TailSignature,
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
		{
			name: "a Von: sentence spanning two lines is prose",
			body: "Von: der Messe habe ich drei Kontakte mitgebracht.\nAlle drei wollen ein Angebot.",
			main: "Von: der Messe habe ich drei Kontakte mitgebracht.\nAlle drei wollen ein Angebot.",
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SplitEmailBody(tc.body)
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
			if got := EmailSummaryText(tc.body); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
