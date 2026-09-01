// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "testing"

// The mask has to be recognisable to its owner and useless to anybody
// else who has found the link.
func TestMaskedEmailKeepsTheFirstRuneAndTheDomain(t *testing.T) {
	if got := MaskEmail("marcus@example.com"); got != "m•••••@example.com" {
		t.Errorf("MaskEmail = %q, want m•••••@example.com", got)
	}
}

// A mask whose length tracked the address would hand back how long the
// local part is, which is a fact about the address nobody asked to share.
func TestMaskedEmailHidesTheLocalPartLength(t *testing.T) {
	short := MaskEmail("jo@example.com")
	long := MaskEmail("jonathan-quentin-smith@example.com")
	if short != long {
		t.Errorf("two local parts masked differently: %q vs %q", short, long)
	}
}

func TestMaskedEmailOfAMultibyteLocalPart(t *testing.T) {
	if got := MaskEmail("ünal@x.de"); got != "ü•••••@x.de" {
		t.Errorf("MaskEmail = %q, want ü•••••@x.de", got)
	}
}

// Nothing recognisable, nothing shown — never a bare domain or a stray
// bullet string that reads like a real address.
func TestMaskedEmailOfAnUnusableAddressIsEmpty(t *testing.T) {
	for _, addr := range []string{"", "   ", "not-an-address", "@example.com", "trailing@"} {
		if got := MaskEmail(addr); got != "" {
			t.Errorf("MaskEmail(%q) = %q, want empty", addr, got)
		}
	}
}

// The question the raw consent state cannot answer on its own. 'unknown'
// is off on a lane that runs on consent and ON for one that does not, and
// reading it without the class is what called a live lane "not subscribed".
func TestChoiceReadsUnknownAgainstTheClass(t *testing.T) {
	cases := []struct {
		name  string
		state string
		class Class
		want  Choice
	}{
		{"marketing nobody opted into is off", "unknown", ClassMarketing, ChoiceOptedOut},
		{"direct correspondence nobody objected to is on", "unknown", ClassBusinessCorrespondence, ChoiceNoObjection},
		{"transactional is on", "unknown", ClassTransactional, ChoiceNoObjection},
		{"an explicit grant is an opt-in", "granted", ClassMarketing, ChoiceOptedIn},
		{"a withdrawal wins on any class", "withdrawn", ClassBusinessCorrespondence, ChoiceOptedOut},
		{"a withdrawal wins on transactional too", "withdrawn", ClassTransactional, ChoiceOptedOut},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := choiceOf(c.state, c.class); got != c.want {
				t.Errorf("choiceOf(%q, %q) = %q, want %q", c.state, c.class, got, c.want)
			}
		})
	}
}

// A purpose named twice in one body is a client bug, and the safe reading
// of it on a consent surface is the suppressing one — never request order,
// which decides it by accident.
func TestASaveSettlesADuplicatePurposeTowardTheWithdrawal(t *testing.T) {
	out := settleTowardWithdrawal([]PreferenceChoiceInput{
		{PurposeKey: "newsletter", State: StateGranted},
		{PurposeKey: "newsletter", State: StateWithdrawn},
	})
	if len(out) != 1 {
		t.Fatalf("got %d choices, want the duplicate collapsed", len(out))
	}
	if out[0].State != StateWithdrawn {
		t.Errorf("state = %q, want withdrawn", out[0].State)
	}
}

// The same, with the grant listed second — order must not decide it.
func TestADuplicateSettlesTheSameWayInEitherOrder(t *testing.T) {
	out := settleTowardWithdrawal([]PreferenceChoiceInput{
		{PurposeKey: "newsletter", State: StateWithdrawn},
		{PurposeKey: "newsletter", State: StateGranted},
	})
	if len(out) != 1 || out[0].State != StateWithdrawn {
		t.Errorf("got %+v, want one withdrawn choice", out)
	}
}
