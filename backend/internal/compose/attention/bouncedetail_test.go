// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which address is dead, on the row that reports the failure.
//
// The card names a person and a subject. A rep opening a contact who carries
// three addresses cannot tell from that which mailbox refused, so the row
// reports a failure and leaves the fix to guesswork — the defect this lane was
// built to remove, one step further along.

import "testing"

func TestABouncedSendNamesTheAddressThatRefusedIt(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		send BouncedSend
		want string
	}{
		"both": {
			BouncedSend{Recipient: "dana@turbinenbau.de", Reason: "550 no such user"},
			"dana@turbinenbau.de — 550 no such user",
		},
		// The provider said nothing usable. The address alone still tells the
		// reader which mailbox to fix, which is the move.
		"address only": {
			BouncedSend{Recipient: "dana@turbinenbau.de"},
			"dana@turbinenbau.de",
		},
		// A send carrying no recipient — an older row, or one stored without
		// one. The reason stands alone rather than the card claiming to know
		// where it was aimed.
		"reason only": {
			BouncedSend{Reason: "550 no such user"},
			"550 no such user",
		},
		// Neither: no line at all, so the card draws nothing rather than an
		// empty supporting row under the subject.
		"neither": {BouncedSend{}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := bounceDetail(tc.send); got != tc.want {
				t.Errorf("the card's line reads %q, want %q", got, tc.want)
			}
		})
	}
}

// And it reaches the row, not just the helper.
//
// bounceItem sets Detail only when the line is non-empty, so a helper returning
// the right string proves nothing about what a reader sees.
func TestTheBounceRowCarriesTheAddress(t *testing.T) {
	t.Parallel()
	item := bounceItem(BouncedSend{
		Subject: "Retrofit quote", Recipient: "dana@turbinenbau.de",
		Reason: "550 no such user",
	})
	if item.Detail == nil {
		t.Fatal("the row carries no supporting line at all")
	}
	if *item.Detail != "dana@turbinenbau.de — 550 no such user" {
		t.Errorf("the row's line reads %q, and a reader cannot see which address is dead", *item.Detail)
	}
	// And a send with nothing to say still draws no line.
	bare := bounceItem(BouncedSend{Subject: "Retrofit quote"})
	if bare.Detail != nil {
		t.Errorf("a send with no address and no reason draws %q under its subject", *bare.Detail)
	}
}
