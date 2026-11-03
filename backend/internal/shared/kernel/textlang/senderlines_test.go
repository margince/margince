// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textlang_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const quotedSellerSignature = "Hi Alex, see below.\n\nSam Seller\nHead of Sales\n+49 30 1111111"

func TestSenderLinesStopAtAnUnmarkedQuote(t *testing.T) {
	reply := "From: Alex Buyer <contact@example.com>\nTo: Sam Seller <sam@example.org>\n\n" +
		"Thanks, works for me.\n\nBest\nAlex\n+49 89 7654321\n\n"
	for name, header := range map[string]string{
		"bold_outlook":        "*From:* Sam Seller <sam@example.org>\n*Sent:* Monday\n\n",
		"double_bold_outlook": "**From:** Sam Seller <sam@example.org>\n**Sent:** Monday\n\n",
		"bold_german":         "*Von:* Sam Seller <sam@example.org>\n*Gesendet:* Montag\n\n",
		"outlook":             "From: Sam Seller <sam@example.org>\nSent: Monday\nTo: Alex Buyer <contact@example.com>\nSubject: Next steps\n\n",
		"german":              "Von: Sam Seller <sam@example.org>\nGesendet: Montag\nAn: Alex Buyer <contact@example.com>\nBetreff: Next steps\n\n",
		"gmail":               "On Mon, 2 Jun 2026 at 10:00, Sam Seller <sam@example.org> wrote:\n\n",
		"forward":             "---------- Forwarded message ---------\nFrom: Sam Seller <sam@example.org>\n\n",
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

func TestSenderLinesEndAtAnOutlookBlockThatFollowsTheEnvelope(t *testing.T) {
	body := "From: Alex Buyer <contact@example.com>\nTo: Sam Seller <sam@example.org>\n" +
		"From: Sam Seller <sam@example.org>\nSent: Monday, June 2, 2026 10:00 AM\n" +
		"To: Alex Buyer <contact@example.com>\nSubject: Next steps\n\n" + quotedSellerSignature
	if got := textlang.SenderLines(body); len(got) != 0 {
		t.Fatalf("sender lines = %q, want none: everything below the second From: is quoted", got)
	}
	if got := textlang.NewTextOnly(body); strings.Contains(got, "Head of Sales") {
		t.Fatalf("NewTextOnly kept the quoted signature: %q", got)
	}
}

// An Outlook block that follows the stored envelope is quoted. The text a proof
// hashes ends there, as every other reader's does.
func TestCurrentMessageEndsAtAnOutlookBlockThatFollowsTheEnvelope(t *testing.T) {
	if got := textlang.CurrentMessage(outlookBlockBody); strings.Contains(got, "Head of Sales") {
		t.Fatalf("CurrentMessage kept the quoted signature: %q", got)
	}
}

// A proof stored with the zero cut hashed the whole of this message. It still
// verifies, because it is read with that cut.
func TestAProofStoredUnderTheEarlierCutStillVerifies(t *testing.T) {
	stored, ok := values.ProveQuote(strings.TrimSpace(outlookBlockBody), "Head of Sales")
	if !ok {
		t.Fatal("the fixture's earlier-cut proof could not be made")
	}
	if quote, ok := textlang.VerifySenderQuote(outlookBlockBody, stored); !ok || quote != "Head of Sales" {
		t.Fatalf("a proof stored under the earlier cut read %q, %v", quote, ok)
	}
}

func TestANewProofNamesItsCutAndVerifies(t *testing.T) {
	body := "Thanks, I will send the signed order by Friday.\n\nBob Contact"
	proof, ok := textlang.ProveSenderQuote(body, "send the signed order by Friday")
	if !ok || proof.Cut != textlang.QuoteCutSecondSender {
		t.Fatalf("proof = %+v, %v; want one made under today's cut", proof, ok)
	}
	if quote, ok := textlang.VerifySenderQuote(body, proof); !ok || quote != "send the signed order by Friday" {
		t.Fatalf("a new proof read %q, %v", quote, ok)
	}
}

func TestAProofUnderAnUnknownCutVerifiesNothing(t *testing.T) {
	body := "Thanks, I will send the signed order by Friday."
	proof, ok := textlang.ProveSenderQuote(body, "send the signed order")
	if !ok {
		t.Fatal("the fixture's proof could not be made")
	}
	proof.Cut = textlang.QuoteCutSecondSender + 1
	if quote, ok := textlang.VerifySenderQuote(body, proof); ok {
		t.Fatalf("a proof under an unknown cut verified as %q", quote)
	}
}

const outlookBlockBody = "From: Alex Buyer <contact@example.com>\nTo: Sam Seller <sam@example.org>\n" +
	"From: Sam Seller <sam@example.org>\nSent: Monday\nTo: Alex Buyer <contact@example.com>\n\n" + quotedSellerSignature

func TestAFromSentenceAfterTheEnvelopeIsNotAQuote(t *testing.T) {
	body := "From: Alex Buyer <contact@example.com>\nTo: Sam Seller <sam@example.org>\n" +
		"From: our finance team's perspective this is the right call\n\nPlease approve the budget."
	if got := textlang.NewTextOnly(body); !strings.Contains(got, "Please approve the budget.") {
		t.Fatalf("a prose line opening with From: cut the message: %q", got)
	}
}

func TestMessageOpeningStopsAtAnOutlookBlockThatFollowsTheEnvelope(t *testing.T) {
	body := "From: Alex Buyer <contact@example.com>\nTo: Sam Seller <sam@example.org>\n\nThanks, works for me.\n\n" +
		"From: Sam Seller <sam@example.org>\nSent: Monday, June 2, 2026 10:00 AM\n" +
		"To: Alex Buyer <contact@example.com>\nSubject: Next steps\n\n" + quotedSellerSignature
	if got := textlang.MessageOpening(body, 800); got != "Thanks, works for me." {
		t.Fatalf("opening = %q, want only the reply above the Outlook block", got)
	}
	direct := "From: Alex Buyer <contact@example.com>\nTo: Sam Seller <sam@example.org>\n" +
		"From: Sam Seller <sam@example.org>\nSent: Monday\nTo: Alex Buyer <contact@example.com>\n\n" + quotedSellerSignature
	if got := textlang.MessageOpening(direct, 800); got != "" {
		t.Fatalf("opening = %q, want nothing from an envelope followed directly by a quote", got)
	}
}
