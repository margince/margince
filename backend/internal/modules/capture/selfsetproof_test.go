// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

// The self set answers two questions and only one of them grants anything.
//
// "Is this me" settles filing: a seat's own address is not their own
// counterparty. "Did this reach my mailbox" settles whether an incumbent
// activity — possibly a COLLEAGUE's held message — may be written against, and
// a message's identity is a Message-ID the sender types. So the second question
// may only be answered by an address somebody proved control of: the mailbox a
// provider attested at grant, or one the seat declared about themselves.
//
// The seat's login address is neither. Nobody proves control of it; the product
// knows it because that is who signed in. Admitting it to the delivery question
// would let a message that merely NAMES it stand as proof of delivery.
func TestALoginAddressNamesTheSeatWithoutProvingDelivery(t *testing.T) {
	t.Parallel()
	const (
		mailbox = "owner@myco.example"
		login   = "owner@corp.example"
	)
	self := NewSelfSetWithIdentityOnly([]string{mailbox}, []string{login}, nil)

	if !self.Covers(login) {
		t.Error("the login address must read as the seat: it is who they signed in as")
	}
	if self.CoversAddressExactly(login) {
		t.Error("the login address must NOT prove delivery — nobody proved the seat controls that mailbox")
	}
	// The attested mailbox answers both, which is what makes it different in
	// kind rather than merely different in origin.
	if !self.Covers(mailbox) || !self.CoversAddressExactly(mailbox) {
		t.Error("the connected mailbox answers both questions")
	}
}

// A seat whose only address is their login is not an empty set. Reading it as
// empty would skip every gate that guards on it, which fails in the direction
// that files a person as their own counterparty again.
func TestASeatKnownOnlyByTheirLoginIsNotAnEmptySet(t *testing.T) {
	t.Parallel()
	self := NewSelfSetWithIdentityOnly(nil, []string{"owner@corp.example"}, nil)
	if self.Empty() {
		t.Error("a seat with a login address is somebody, and the gates must run for them")
	}
}

// The same address arriving both ways keeps the stronger claim. Holding one
// address in two sets would leave a later reader asking which wins.
func TestAnAddressProvenAndAlsoTheLoginStaysProven(t *testing.T) {
	t.Parallel()
	const both = "owner@myco.example"
	self := NewSelfSetWithIdentityOnly([]string{both}, []string{both}, nil)
	if !self.CoversAddressExactly(both) {
		t.Error("an address the provider attested does not stop being proof because it is also the login")
	}
}
