// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the vCard create precheck refuses while the decision can still be
// taken back, and what it leaves alone.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
)

func stagedCard(t *testing.T, entry people.VCardEntry) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(vcardCreateProposal{Entry: entry, FullName: entry.FullName})
	if err != nil {
		t.Fatalf("staging the card: %v", err)
	}
	return payload
}

// A number the writer will not store must be refused at DECIDE time. Approved,
// it fails after the decision committed: the redemption rolls back, the row is
// marked effect_failed, and the decider finds out from the did-not-run lane
// instead of from the decision they were making.
func TestACardCarryingAnUnstorableContactIsRefusedWhileItIsStillDecidable(t *testing.T) {
	cases := []struct {
		name  string
		entry people.VCardEntry
	}{
		{
			name: "a number that is not one",
			entry: people.VCardEntry{
				FullName: "Ana Ionescu",
				Phones:   []people.VCardChannel{{Value: "not a phone number", Kind: "work"}},
			},
		},
		{
			name: "an address that is not one",
			entry: people.VCardEntry{
				FullName: "Ana Ionescu",
				Emails:   []people.VCardChannel{{Value: "ana at example dot com", Kind: "work"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := vcardCreatePrecheck()(context.Background(), stagedCard(t, tc.entry), nil)
			if err == nil {
				t.Fatal("the precheck admitted a card the create will refuse, so the failure moves to after the decision")
			}
			// The decider is told what is wrong with the card, not merely that
			// something is: this message is what they act on.
			if !strings.Contains(err.Error(), "contact detail that cannot be stored") {
				t.Errorf("refusal reads %q, which does not say what a decider should do about it", err)
			}
		})
	}
}

// The card the reviewer is looking at is not rewritten by being checked. The
// parse the create runs normalises in place, and a precheck that normalised
// the staged row would change what the decider approved after they read it.
func TestTheCheckLeavesTheStagedCardExactlyAsItWas(t *testing.T) {
	entry := people.VCardEntry{
		FullName: "Ana Ionescu",
		Emails:   []people.VCardChannel{{Value: "  Ana.Ionescu@Example.COM  ", Kind: "work"}},
		Phones:   []people.VCardChannel{{Value: "+40 21 555 0100", Kind: "work"}},
	}
	staged := stagedCard(t, entry)
	before := append(json.RawMessage(nil), staged...)

	if err := vcardCreatePrecheck()(context.Background(), staged, nil); err != nil {
		t.Fatalf("a storable card was refused: %v", err)
	}

	if string(staged) != string(before) {
		t.Fatalf("the precheck rewrote the staged payload:\n before: %s\n  after: %s", before, staged)
	}
	if entry.Emails[0].Value != "  Ana.Ionescu@Example.COM  " {
		t.Errorf("the entry's own address was normalised to %q by a check that should only read it", entry.Emails[0].Value)
	}
}

// The edited payload is the one checked, because the modify-then-approve arm
// is exactly where a decider can introduce a value the create refuses.
func TestAnEditedCardIsTheOneChecked(t *testing.T) {
	good := stagedCard(t, people.VCardEntry{FullName: "Ana Ionescu"})
	edited := stagedCard(t, people.VCardEntry{
		FullName: "Ana Ionescu",
		Phones:   []people.VCardChannel{{Value: "not a phone number", Kind: "work"}},
	})

	if err := vcardCreatePrecheck()(context.Background(), good, edited); err == nil {
		t.Fatal("the precheck read the staged card and ignored the edit that broke it")
	}
}
