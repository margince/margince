// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// This request carries any subset of three independent facts, so what an
// omitted field means is the contract. Read as a value instead of a pointer,
// each omission becomes a write of Go's zero.
func TestASaveCarriesOnlyTheFieldsItNames(t *testing.T) {
	t.Parallel()
	answer := false
	in := saveLinkedInAccountInput(crmcontracts.SaveLinkedInAccountRequest{
		ReachVisibleToTeam: &answer,
	})

	if in.ProfileURL != nil {
		t.Errorf("an unmentioned profile URL arrived as %q, want nil so the stored one stands",
			*in.ProfileURL)
	}
	if in.ReachVisibleToTeam == nil || *in.ReachVisibleToTeam {
		t.Errorf("the answer given is %v, want false carried through", in.ReachVisibleToTeam)
	}
	if in.Connected {
		t.Error("an unmentioned authorization reads as recorded")
	}
}

// Empty is a member clearing the field, which is a different request from not
// mentioning it. One pointer has to tell the two apart.
func TestAnEmptyProfileURLIsStillAnAnswer(t *testing.T) {
	t.Parallel()
	empty := ""
	in := saveLinkedInAccountInput(crmcontracts.SaveLinkedInAccountRequest{
		ProfileUrl: &empty,
	})

	if in.ProfileURL == nil {
		t.Fatal("emptying the field reads as not mentioning it")
	}
	if *in.ProfileURL != "" {
		t.Errorf("profile URL = %q, want empty", *in.ProfileURL)
	}
}

// The reach answer has to cross to the wire. Otherwise the control that sets
// it reads its own default back, and a member who withheld their reach sees
// it on.
func TestTheAccountReportsTheReachAnswer(t *testing.T) {
	t.Parallel()
	for name, withheld := range map[string]bool{
		"a member who shares their reach":   true,
		"a member who withheld their reach": false,
	} {
		wire := linkedInAccountWire(LinkedInAccount{ReachVisibleToTeam: withheld})
		if wire.ReachVisibleToTeam == nil {
			t.Fatalf("%s: the answer is absent from the wire", name)
		}
		if *wire.ReachVisibleToTeam != withheld {
			t.Errorf("%s: wire says %v, want %v", name, *wire.ReachVisibleToTeam, withheld)
		}
	}
}

// Connected is derived from the timestamp rather than stored beside it. One
// fact read two ways drifts; read one way it cannot.
func TestConnectedFollowsTheAuthorizationTimestamp(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

	if linkedInAccountWire(LinkedInAccount{}).Connected {
		t.Error("an account with no authorization reads as connected")
	}
	connected := linkedInAccountWire(LinkedInAccount{ConnectedAt: &at})
	if !connected.Connected {
		t.Error("an account with an authorization reads as not connected")
	}
	if connected.ConnectedAt == nil || !connected.ConnectedAt.Equal(at) {
		t.Errorf("connected at %v, want %v", connected.ConnectedAt, at)
	}
}
