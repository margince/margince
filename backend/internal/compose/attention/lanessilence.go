// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two lanes that report a SILENCE: a deal nobody is moving, and a contact
// nobody is talking to. Two lanes rather than more rows in one because a contact
// carrying no open deal never reaches the risk lane, and those are exactly the
// relationships that lapse unnoticed.
//
// Both must answer more than the clock: a silence has to say what it COSTS, or
// every row reads alike and the rep stops reading the lane. Optional as the
// other lanes are — nil is a different fact from a rep whose deals all move.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// AtRisk is the open deals going quiet, or already past their expected close. It
// reads the SAME candidate engine the whats_slipping tool reads, at a shorter
// idle window; a second at-risk rule here would disagree in front of a rep.
type AtRisk interface {
	// The bool reports the read was CUT. It travels beside the rows rather than
	// being inferred from their number because this lane FILTERS after it scans,
	// so ten rows can be the survivors of a bounded fifty. Counting rows against
	// the bound would read a truncated scan as a complete one — under-reporting,
	// the one direction a work figure must never fail in.
	Quiet(ctx context.Context) ([]RiskyDeal, bool, error)
}

// RiskyDeal is one deal the pipeline should worry about, and the ground it is
// worried on. Both flags travel because they are different warnings: untouched
// is neglected, past its close date is late whether touched or not, and a card
// collapsing them would say "at risk" and leave the rep to guess which.
type RiskyDeal struct {
	CloseDateProvisional *bool
	ForecastCategory     *string
	DealID               ids.UUID
	Name                 string
	// The card's facts, carried so the client draws value, stage and
	// ownership without a second read per row. All optional: a deal can be
	// ownerless, unpriced, or an overlay mirror with no native stage.
	StageID     *ids.UUID
	OwnerID     *ids.UUID
	AmountMinor *int64
	Currency    *string
	// QuietDays is how long the deal has been idle, which is the number the
	// card says out loud. Zero for a deal admitted only by its close date.
	QuietDays int
	// CloseOverdue is set when the expected close date has already passed.
	CloseOverdue      bool
	ExpectedCloseDate *time.Time
	// The account has a committee and nobody engaged holds the champion seat —
	// a different problem from a deal nobody outside has touched, needing a
	// different move. Nil rather than false when the reader cannot see every
	// seat, or there is no committee: both would render as "nobody is carrying
	// this", and a champion the reader may not read is still a champion.
	NoChampion *bool
}

// Decay is the reader's own relationships that have gone silent. It derives the
// silence through the same §4 change engine the contact's own page reads, so
// there is one quiet rule in the product rather than a second threshold here.
type Decay interface {
	Lapsed(ctx context.Context) ([]QuietRelationship, error)
}

// QuietRelationship is one contact this reader has stopped talking to.
type QuietRelationship struct {
	ContactID ids.UUID
	Name      string
	// From the derivation rather than the projection's own last_at, so the card
	// and the contact's page agree.
	QuietDays int
	// When they last spoke, so the card can date the silence, not only measure it.
	LastAt time.Time
	// What the relationship was WORTH before it went quiet, scored through the
	// same relstrength.Compute the contact's page reads. The band comes back off
	// that call rather than being derived here, so there is no second way to turn
	// a score into the word a rep reads. Without it every lapsed contact reads
	// alike, and a strong relationship going quiet is the row that most deserves
	// telling apart from a cc who has drifted.
	Strength relstrength.Score
	// Whether money this reader can see still rests on this contact: a silence
	// with a deal behind it is not the same work as one without.
	HasOpenDeal bool
}
