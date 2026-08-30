// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a close-date correction IS, and the seams the composition root fills to
// raise one: the staged payload a human weighs, the kind it is staged under, and
// the readers that let the nightly sweep ask the approvals engine what it has
// already proposed and what a human has already refused.
//
// This lives apart from the sweep because the shape outlives any one pass: the
// confirm effect unmarshals this payload, the queue renders it, and the staging
// identity is drawn from its fields. The sweep is one caller of it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CloseDateCorrectionKind is the approvals staging kind the 🟡/🔻 tiers
// surface through; its decision grant lives in the approvals module and
// its confirm effect is injected at the composition root.
const CloseDateCorrectionKind = "close_date_correction"

// CorrectionStager is the approvals seam the composition root fills
// (a module never imports a sibling): stage a 🟡 confirm-the-real-date
// proposal, and ask whether one is already pending so a nightly sweep —
// whose proposed date moves with "today" — cannot stack duplicates.
type CorrectionStager interface {
	HasPendingCorrection(ctx context.Context, dealID ids.UUID) (bool, error)
	StageCorrection(ctx context.Context, dealID ids.UUID, targetVersion int64, summary string, proposal CloseDateCorrection) error
	// RefusedCloseDate reports whether a human has already turned down the
	// correction this probe describes.
	//
	// The staging memory cannot answer it alone. The sweep writes its new date
	// onto the deal BEFORE staging, so the standing date the identity is drawn
	// from has already moved by the time the proposal is built, and a refusal
	// recorded last night matches nothing tonight.
	//
	// The probe carries the judgment rather than a bare date, because what makes
	// two corrections the same question belongs to this module and the adapter
	// only walks the payloads.
	RefusedCloseDate(ctx context.Context, dealID ids.UUID, proposed RefusalProbe) (bool, error)
}

// QuietReviewReader reads one deal's correspondence UNDER ITS OWNER'S OWN
// AUTHORITY, and that principal is the whole security argument for the
// gone-quiet review.
//
// The sweep runs as a system principal, which auth.Require passes
// unconditionally and which no row scope bounds. A counterparty resolved under
// it is resolvable whoever they are — and the review writes that name into
// proposed_change, where anyone holding deal:update on the target reads it
// later. An unbounded read frozen into a stored payload is a disclosure no
// read-side gate can undo, so the read runs as the person the card is for: the
// deal's owner, resolved per deal.
//
// The composition root fills this because resolving an authority means reading
// app_user, which this module may not do.
//
// Best-effort by contract. A deal with no owner, or one whose owner is no
// longer live, yields no facts and the review falls back to a reason carrying
// no name. Absence of authority is denial rather than empty permission, and a
// deal's date hygiene is not a reason to fail the pass.
type QuietReviewReader interface {
	ReadForOwner(ctx context.Context, dealID ids.DealID) (QuietFacts, QuietNames, error)
}

// CloseDateCorrection is the staged proposed_change payload: everything
// a human needs to confirm (or edit) the replacement date, and the
// confirm effect needs to apply it.
type CloseDateCorrection struct {
	DealID ids.DealID `json:"deal_id"`
	// ExpectedCloseDate is the proposed date, date-only wire form.
	ExpectedCloseDate string          `json:"expected_close_date"`
	PreviousCloseDate *string         `json:"previous_close_date"`
	Flags             []CloseDateFlag `json:"flags"`
	// Basis is the plain-language derivation of the proposed date — the
	// "no mystery number" duty (P6) applied to a guess.
	Basis string `json:"basis"`
	// StandingCloseDate is the date the REP owns, in the form a staging identity
	// can be keyed on: never absent, and a sentinel where there is no such date.
	//
	// It exists beside the nullable PreviousCloseDate rather than replacing it
	// because a staging identity must be a string the payload carries with the
	// same value (canonicalIdentity enforces both), and an absent key never
	// matches. The card renders the nullable one, which reads as "no previous
	// date" rather than as a sentinel word.
	StandingCloseDate string `json:"standing_close_date"`
}

// standingNoDate is a deal holding no close date at all — a real case, the
// `missing` flag exists for it. A value rather than an absent key, because jsonb
// containment never matches a key the payload does not carry, so such a deal's
// proposals would otherwise carry no rejection memory whatever. It cannot
// collide with a real date: every one of those is formatted YYYY-MM-DD.
const standingNoDate = "none"

// RefusalProbe is one proposed correction, in the terms that decide whether a
// human has already answered it.
type RefusalProbe struct {
	// Proposed is the date this pass wants to put on the deal.
	Proposed string
	// AfterHumanEdit says the deal is standing at a date a person chose, so this
	// is a question about THEIR date rather than about the machine's own guess.
	AfterHumanEdit bool
}

// ProbeFor reads a proposal and the state it was raised from into the question
// it asks.
func ProbeFor(proposal CloseDateCorrection, provisional bool) RefusalProbe {
	return RefusalProbe{Proposed: proposal.ExpectedCloseDate, AfterHumanEdit: !provisional}
}

// SameQuestionAs reports whether an earlier refused correction already answered
// this one.
//
// The date proposed is what a rep actually said no to, and it is stable across
// nights: the sweep computes it from stage velocity, so a deal whose situation
// has not changed is offered the same date again. That is the resurrection this
// memory exists to stop, and matching on the proposed date alone stops it.
//
// The second term stops it silencing too much. A rep who refuses a date and then
// sets their own has changed the question — the deal now stands at a date a
// person chose, and the sweep's next guess for it can easily be the same
// computed day. Without this, that guess would read as the refused one and the
// rep would never hear that their own date had gone stale.
//
// What it deliberately does NOT use is the standing date. The sweep writes its
// date onto the deal before staging, so what a deal stands at tonight is what
// was proposed last night — every pass would look like a fresh question and
// nothing would be remembered at all.
func (p RefusalProbe) SameQuestionAs(earlier CloseDateCorrection) bool {
	return p.Proposed == earlier.ExpectedCloseDate && !p.AfterHumanEdit
}

// StandingCloseDate spells, for a staging identity, WHICH date this correction
// is about: the one the deal holds and the sweep is asking to replace.
//
// Not the PROPOSED date, which is recomputed against "today" on every pass — an
// identity carrying it makes each night a different question and a rep's "no" is
// forgotten by morning.
//
// It does move when the sweep writes its own guess, and that is what the
// keep-alive branch's separate refusal check exists to cover. Here it is what
// keeps a date the rep sets themselves askable: a new standing date is a new
// question, so one refusal cannot end close-date hygiene on a deal for good.
func StandingCloseDate(current *string) string {
	if current == nil {
		return standingNoDate
	}
	return *current
}

// UnmarshalCloseDateCorrection decodes a staged (possibly human-edited)
// proposal back into the typed form the confirm effect applies.
func UnmarshalCloseDateCorrection(raw json.RawMessage) (CloseDateCorrection, error) {
	var c CloseDateCorrection
	if err := json.Unmarshal(raw, &c); err != nil {
		return CloseDateCorrection{}, fmt.Errorf("close_date_correction payload: %w", err)
	}
	if c.DealID.IsZero() {
		return CloseDateCorrection{}, errors.New("close_date_correction payload names no deal")
	}
	if _, err := time.Parse(time.DateOnly, c.ExpectedCloseDate); err != nil {
		return CloseDateCorrection{}, fmt.Errorf("close_date_correction payload date: %w", err)
	}
	return c, nil
}
