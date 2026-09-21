// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Seat is one contact on the deal, as the card needs them: who they are, what
// role they hold, and whether they have spoken with us both ways inside the
// engagement window.
//
// Name is empty when the reader may not read that contact. The seat still
// counts — how many contacts carry a deal is not the secret, only who they are —
// and the card names roles it cannot name contacts for.
type Seat struct {
	Role    string
	Name    string
	Engaged bool
	// ContactID names the contact behind the seat. Zero when the reader may
	// not read them, which is the same case that leaves Name empty.
	ContactID ids.UUID
	// Attachable says this reader may FILE WORK against the contact, which is
	// a different grant from reading them: a share marked read-only lets
	// somebody see a contact and not add to their record
	// (auth.EnsureAttachTarget). A move that links a task to a contact the
	// reader may not attach to is a button that fails after the click, so the
	// link is withheld and the advice is offered without it.
	Attachable bool
}

// bestSeat is who to approach, best answer first, in the same order the
// opening move uses. Extracted so the two rungs share one idea of who matters
// on a deal rather than growing two.
//
// Held by: TestTheMeetingRequestTakesTheBestAvailableRole and
// TestTheFirstMoveTakesTheBestAvailableRole (move_test.go)
//
// A role this vocabulary cannot order is no answer rather than an arbitrary
// one: picking a stranger and putting their name in an instruction is worse
// than naming nobody.
func bestSeat(seats []Seat) (Seat, bool) {
	for _, role := range openingRoles {
		if seat, ok := namedRole(seats, role); ok {
			return seat, true
		}
	}
	return Seat{}, false
}

// seatWords names a seat for a sentence: the contact where the reader may know
// them, the role alone where they may not.
//
// The role is never dropped. "Annabelle Malherbe" tells a reader who; "the
// champion" tells them why it is that one, and a card that has both says both.
func seatWords(seat Seat) string {
	if seat.Name == "" {
		return "the " + roleWord(seat.Role)
	}
	return seat.Name + ", the " + roleWord(seat.Role)
}

// SeatReader answers who sits on a deal.
//
// A port rather than a store call, because the answer is assembled by
// compose/network's CoverageFor, and a module may not import a compose
// subpackage (ADR-0054 §3). The edge is injected in compose, which is where
// the one existing assembler is bound — writing a second seat read here would
// be the duplicate the coverage seam exists to prevent, and it would be the
// copy without CoverageFor's edge admission.
//
// A reader refused the stakeholder edge gets NO seats and no error: the card
// then says nothing about who is on the deal, which is true for them. That is
// the same shape CoverageFor already uses for a withheld section.
type SeatReader func(ctx context.Context, dealID ids.DealID, now time.Time) ([]Seat, error)

// namedRole returns the first seat holding the role, preferring one this
// reader may name: a role the card can attach a contact to is worth more than
// the same role as an anonymous count, and an unnamed seat still proves the
// role is filled.
func namedRole(seats []Seat, role string) (Seat, bool) {
	var unnamed Seat
	var found bool
	for _, s := range seats {
		if s.Role != role {
			continue
		}
		if s.Name != "" {
			return s, true
		}
		if !found {
			unnamed, found = s, true
		}
	}
	return unnamed, found
}
