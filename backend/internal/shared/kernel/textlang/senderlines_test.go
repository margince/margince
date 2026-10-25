// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textlang_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const quotedSellerSignature = "Hi Alex, see below.\n\nSam Seller\nHead of Sales\n+49 30 1111111"

func TestSenderLinesStopAtAnUnmarkedQuote(t *testing.T) {
	reply := "From: Alex Buyer <contact@example.com>\nTo: Sam Seller <sam@example.org>\n\n" +
		"Thanks, works for me.\n\nBest\nAlex\n+49 89 7654321\n\n"
	for name, header := range map[string]string{
		"outlook": "From: Sam Seller <sam@example.org>\nSent: Monday\nTo: Alex Buyer <contact@example.com>\nSubject: Next steps\n\n",
		"german":  "Von: Sam Seller <sam@example.org>\nGesendet: Montag\nAn: Alex Buyer <contact@example.com>\nBetreff: Next steps\n\n",
		"gmail":   "On Mon, 2 Jun 2026 at 10:00, Sam Seller <sam@example.org> wrote:\n\n",
		"forward": "---------- Forwarded message ---------\nFrom: Sam Seller <sam@example.org>\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(textlang.SenderLines(reply+header+quotedSellerSignature), "\n")
			if want := "Thanks, works for me.\nBest\nAlex\n+49 89 7654321"; got != want {
				t.Fatalf("sender lines = %q, want %q", got, want)
			}
		})
	}
}

func TestSenderLinesKeepABottomPostedSignature(t *testing.T) {
	body := "On Tue, Sam Seller wrote:\n> Can you confirm?\n> Sam Seller\n> +49 30 1111111\n\nConfirmed.\n\nBob Contact\nCTO, Acme GmbH"
	got := strings.Join(textlang.SenderLines(body), "\n")
	if want := "Confirmed.\nBob Contact\nCTO, Acme GmbH"; got != want {
		t.Fatalf("sender lines = %q, want %q", got, want)
	}
}

func TestSenderLinesOfAForwardHoldNoneOfTheOriginal(t *testing.T) {
	body := "-----Original Message-----\nFrom: Sam Seller <sam@example.org>\n\n" + quotedSellerSignature
	if got := textlang.SenderLines(body); len(got) != 0 {
		t.Fatalf("a bare forward kept %q as its sender's words", got)
	}
}

func TestSenderLinesCannotReadAnAnswerBelowAnUnmarkedQuote(t *testing.T) {
	// An unmarked quote has no end, so an answer under it reads as the quoted mail.
	body := "Hello\n\nFrom: Sam Seller <sam@example.org>\nSent: Monday\n\nCan you confirm?\n\nConfirmed.\n\nBob Contact\nCTO"
	if got := textlang.SenderLines(body); !slices.Equal(got, []string{"Hello"}) {
		t.Fatalf("sender lines = %q, want only the line above the quote", got)
	}
}
