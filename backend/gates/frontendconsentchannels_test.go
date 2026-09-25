// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// Every channel the consent guard can answer on has a row on the contact rail.
//
// The rail draws one row per transport and names the channel each row answers
// for. An entry arriving on a channel no row names does not fall through to a
// default — it is drawn NOWHERE, and a permission a rep never sees is the one
// shape of this defect nobody reports. The guard is advisory, so no downstream
// call fails to mark the omission either.
//
// TypeScript cannot see this. Comparing a channel against a literal the union
// does not carry is a compile error already, so a RENAME is held; a channel the
// contract GAINS matches none of the rail's tests and silently disappears,
// which is exactly the direction a census must not fail in.
//
// The corpus is the generated contract enum rather than a list written here. A
// list would be a third spelling of one vocabulary, and the copy that stopped
// matching would let a channel ship with no row while this gate still passed.
//
// Both directions: a channel the contract carries and the rail does not name
// fails, and so does one the rail names that the contract has dropped — a
// selection matching nothing reads to the next author as a row somebody keeps.

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

const (
	consentRailSurface = "../frontend/src/screens/contactconsentpanel.tsx"
	guardChannelType   = "ContactConsentGuardEntryChannel"
)

// railChannelTest captures the channel a row's selection compares against.
//
// Scoped to the rail alone, which is what makes `.channel ===` safe to match:
// swept across the tree the same field name belongs to half a dozen unrelated
// schemas — a deal's verdict, a memory row, an approval — and the gate would
// spend its life ratifying them.
var railChannelTest = regexp.MustCompile(`\.channel\s*===\s*"([^"]+)"`)

func TestEveryGuardChannelHasARailRow(t *testing.T) {
	t.Parallel()

	channels := goConstSet(t, "internal/contracts", guardChannelType)
	// Under-recognition is the failure that reports PASS: a derivation that
	// stopped resolving hands this test an empty corpus and then agrees with
	// every rail there could be. Mail and phone are both in the contract today.
	if len(channels) < 2 {
		t.Fatalf("derived %d channel(s) from %s, want at least the mail and phone the contract "+
			"carries: the census has stopped seeing its subject (%v)",
			len(channels), guardChannelType, channels)
	}

	raw, err := os.ReadFile(consentRailSurface)
	if err != nil {
		t.Fatalf("reading the consent rail: %v", err)
	}
	var named []string
	for _, match := range railChannelTest.FindAllStringSubmatch(string(raw), -1) {
		named = append(named, match[1])
	}
	// The other way the walk goes blind: the rail still draws rows, this gate
	// just stopped recognising how it picks them, and an empty set matches
	// nothing to report.
	if len(named) == 0 {
		t.Fatalf("%s selects no row by channel: either the rail stopped drawing a row per "+
			"transport, or this gate has stopped reading how it picks them", consentRailSurface)
	}

	for _, channel := range channels {
		if !slices.Contains(named, channel) {
			t.Errorf("the guard answers on %q and the rail names no row for it, so an entry on "+
				"that channel is drawn nowhere at all: give it a row in %s",
				channel, consentRailSurface)
		}
	}
	for _, channel := range named {
		if !slices.Contains(channels, channel) {
			t.Errorf("%s selects a row on channel %q, which the contract does not carry: the "+
				"test matches nothing and reads as a row somebody maintains",
				consentRailSurface, channel)
		}
	}
}
