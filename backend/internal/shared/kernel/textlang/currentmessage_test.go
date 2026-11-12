// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textlang_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestCurrentMessageWithholdsForwardedWords(t *testing.T) {
	for _, boundary := range []string{
		"---------- Forwarded message ---------",
		"-----Original Message-----",
		"----- Weitergeleitete Nachricht -----",
		"Anfang der weitergeleiteten E-Mail:",
		"Begin forwarded message:",
		"Thư được chuyển tiếp:",
		"---------- Tin nhắn được chuyển tiếp ---------",
	} {
		t.Run(boundary, func(t *testing.T) {
			body := boundary + "\nFrom: Other <other@example.test>\n\nI will send the quote."
			if got := textlang.CurrentMessage(body); got != "" {
				t.Fatalf("forward alone retained %q", got)
			}
			intro := "For your information."
			if got := textlang.CurrentMessage(intro + "\n\n" + body); got != intro {
				t.Fatalf("current words = %q, want %q", got, intro)
			}
		})
	}
}

func TestCurrentMessageStopsAtHeadersAfterTheSendersWords(t *testing.T) {
	body := "FYI\n\nFrom: Other <other@example.test>\nDate: Monday\n\nI will send the quote."
	if got := textlang.CurrentMessage(body); got != "FYI" {
		t.Fatalf("forward retained %q", got)
	}
}

func TestCurrentMessageKeepsWordsBelowATemplateDivider(t *testing.T) {
	body := "Update\n_____\nI will send the quote."
	if got := textlang.CurrentMessage(body); got != body {
		t.Fatalf("template lost current words: %q", got)
	}
}

func TestCurrentMessageStopsAtAnAttributionLine(t *testing.T) {
	for _, attribution := range []string{
		"On Mon, 10 Nov 2026 at 09:00, Anna Berg <anna@example.test> wrote:",
		"Am 10.11.2026 um 09:00 schrieb Anna Berg <anna@example.test>:",
		"Vào Th 2, 10 thg 11, 2026 vào lúc 09:00 Anna Berg <anna@example.test> đã viết:",
	} {
		t.Run(attribution, func(t *testing.T) {
			reply := "Thanks, we will wait for your answer."
			body := reply + "\n\n" + attribution + "\nWe will not go ahead with this project."
			if got := textlang.CurrentMessage(body); got != reply {
				t.Fatalf("current words = %q, want %q", got, reply)
			}
		})
	}
}
