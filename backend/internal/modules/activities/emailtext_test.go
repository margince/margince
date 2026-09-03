// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

// The cases here are the shared fixtures the frontend splitter is held to as
// well. A case added on one side and not the other is the drift this file and
// gates/frontendemailtext_test.go exist to catch, so the table is written to be
// read from both languages: one body in, one main and one trimmed out.

func TestSplitEmailBodyFindsWhatTheSenderWrote(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		body    string
		main    string
		trimmed string
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
		},
		{
			name:    "a German sign-off near the end closes the message",
			body:    "Danke für das Angebot.\n\nViele Grüße\nAna",
			main:    "Danke für das Angebot.",
			trimmed: "Viele Grüße\nAna",
		},
		{
			name:    "an attribution line travels with the quote it introduces",
			body:    "Ja, gerne.\n\nAm 1. September schrieb Ana:\n> Passt Dienstag?",
			main:    "Ja, gerne.",
			trimmed: "Am 1. September schrieb Ana:\n> Passt Dienstag?",
		},
		{
			name:    "the Outlook block needs its sent-date neighbour",
			body:    "Siehe unten.\n\nVon: Ana Sommer\nGesendet: Montag, 1. September\nAn: Lars\n\nPasst Dienstag?",
			main:    "Siehe unten.",
			trimmed: "Von: Ana Sommer\nGesendet: Montag, 1. September\nAn: Lars\n\nPasst Dienstag?",
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
