// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"time"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// Seat is one person on the deal, as the card needs them: who they are, what
// role they hold, and whether they have spoken with us both ways inside the
// engagement window.
//
// Name is empty when the reader may not read that person. The seat still
// counts — how many people carry a deal is not the secret, only who they are —
// and the card names roles it cannot name people for.
type Seat struct {
	Role    string
	Name    string
	Engaged bool
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
// reader may name: a role the card can attach a person to is worth more than
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
