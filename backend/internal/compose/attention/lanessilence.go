// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two lanes that report a SILENCE, and what each reads it into.
//
// A deal nobody is moving and a person nobody is talking to. They rest on
// different records and warn about different things, which is why they are two
// lanes rather than more rows in one — a contact carrying no open deal never
// reaches the risk lane at all, and those are exactly the relationships that
// lapse without anyone noticing.
//
// What makes them one concept is the question they both have to answer beyond
// the clock: how long is not enough. A silence has to say what it COSTS, or
// every row reads alike and the rep stops reading the lane.
//
// Optional exactly as the other lanes are: nil means this feed derives no such
// answer, which is a different fact from a rep whose deals are all moving.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// AtRisk is the open deals going quiet, or already past their expected close.
//
// The seam behind it reads the SAME candidate engine the whats_slipping tool
// reads, at a shorter idle window. A second at-risk rule living here would be
// two answers to one question, and the two would disagree in front of a rep.
//
// Optional exactly as Commitments is: nil means this feed does not do deal risk,
// which is a different fact from a pipeline with nothing wrong in it.
type AtRisk interface {
	// The bool reports that the read was CUT — the lane scanned to its own bound
	// and deals may sit past it.
	//
	// It travels beside the rows rather than being inferred from their number,
	// because this lane FILTERS after it scans: it returns only what is quiet or
	// overdue out of a bounded sweep, so ten rows can be the survivors of a full
	// fifty. A caller counting rows against the bound would read a truncated
	// scan with few survivors as a complete one — under-reporting, which is the
	// one direction a work figure must never fail in.
	Quiet(ctx context.Context) ([]RiskyDeal, bool, error)
}

// RiskyDeal is one deal the pipeline should worry about, and the ground it is
// worried on.
//
// Both flags travel because they are different warnings: a deal nobody has
// touched is neglected, and a deal past its close date is late whether or not
// anyone touched it. A card that collapsed them would say "at risk" and leave
// the rep to guess which.
type RiskyDeal struct {
	DealID ids.UUID
	Name   string
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
	// NoChampion says the account has a committee and nobody engaged on it
	// holds the champion seat — a deal drifting because nobody inside is
	// arguing for it, which is a different problem from one nobody outside has
	// touched and needs a different move from the rep.
	//
	// Nil rather than false when the reader could not see every seat, or when
	// the deal has no committee at all. Both would render as "nobody is
	// carrying this", and neither is that: a champion the reader may not read
	// is still a champion, and a deal with no seats has no coverage gap to
	// report. A tri-state here is what keeps the lane from turning a boundary
	// into a finding.
	NoChampion *bool
}

// Decay is the reader's own relationships that have gone silent.
//
// A separate lane from AtRisk rather than more rows in it, because the two rest
// on different records and warn about different things: AtRisk is a DEAL nobody
// is moving, and this is a PERSON nobody is talking to. A contact carrying no
// open deal never reaches that lane at all, and those are exactly the
// relationships that lapse without anyone noticing.
//
// The seam behind it derives the silence through the same §4 change engine the
// contact's own page reads, so there is one quiet rule in the product rather
// than a second threshold spelled here.
//
// Optional exactly as the others are: nil means this feed does not derive
// relationship changes, which is a different fact from a rep whose
// relationships are all current.
type Decay interface {
	Lapsed(ctx context.Context) ([]QuietRelationship, error)
}

// QuietRelationship is one contact this reader has stopped talking to.
type QuietRelationship struct {
	PersonID ids.UUID
	Name     string
	// QuietDays is how long the silence has run, which is the number the card
	// says out loud. It comes from the derivation rather than from the
	// projection's own last_at, so the card and the contact's page agree.
	QuietDays int
	// LastAt is when they last spoke, so the card can date the silence rather
	// than only measure it.
	LastAt time.Time
	// Strength is what the relationship was WORTH before it went quiet, scored
	// at the read instant through §4 — relstrength.Compute, which is what the
	// contact's own page reads too. The band comes back off that call rather
	// than being derived here: bucketFor is unexported, so there is no second
	// way to turn a score into the word a rep reads.
	//
	// The lane already holds the edge this is computed from and used to discard
	// it. Without the band every lapsed contact reads alike, and the rep's
	// strongest relationship going quiet is the row that most deserves to be
	// told apart from a cc who has drifted.
	Strength relstrength.Score
	// HasOpenDeal reports whether money this reader can see still rests on this
	// contact. Same fact the waiting lane carries and for the same reason: a
	// silence with a deal behind it is not the same work as one without.
	HasOpenDeal bool
}
