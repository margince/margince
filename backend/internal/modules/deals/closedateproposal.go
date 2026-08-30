// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a close-date correction IS, and the seams the composition root fills to
// raise one: the staged payload a human weighs, the kind it is staged under, and
// the readers that let the nightly sweep ask the approvals engine what it has
// already proposed and what a human has already refused.
//
// This lives apart from the sweep because the shape outlives any one pass: the
// confirm effect unmarshals this payload, the queue renders it, and the
// rejection memory compares it against what the sweep wants to raise tonight.
// The sweep is one caller of it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
	// correction this probe describes. It is the WHOLE of the rejection memory:
	// the staging engine's own declined check cannot express this rule, so the
	// composition root declares no identity for it to enforce.
	//
	// The probe carries the question rather than a bare date, because what makes
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
	// RemainingOpenStages is how far the deal still has to go: the count of open
	// stages at or beyond the one it sits in. It is what the guessed date is
	// computed from, and it is the only part of the question that holds still
	// from one night to the next — see SameQuestionAs.
	//
	// The approvals card never shows it: the kind declares its editable and its
	// displayed fields explicitly, and this is in neither list.
	RemainingOpenStages string `json:"remaining_open_stages"`
}

// StagesRemaining renders the stage distance in the form the payload carries.
// It reads StagesToGo rather than the raw count so the memory's key and the
// date's derivation clamp identically.
func StagesRemaining(openStages int) string {
	return strconv.Itoa(StagesToGo(openStages))
}

// RefusalProbe is tonight's proposed correction plus the date the deal actually
// stands at, which together decide whether a human has already answered it.
type RefusalProbe struct {
	// RemainingOpenStages is the deal's distance from the end, spelled as the
	// payload spells it so the comparison is against what was really stored.
	RemainingOpenStages string
	// StandingCloseDate is the date on the deal right now, before this pass
	// writes anything. It is not compared against another probe's — it is
	// compared against what the REFUSED proposal wanted to put there, which is
	// how "the rep has since set their own date" is recognised.
	StandingCloseDate string
}

// ProbeFor reads tonight's proposal, and the date the deal currently holds, into
// the question it asks.
func ProbeFor(proposal CloseDateCorrection, standing *time.Time) RefusalProbe {
	return RefusalProbe{
		RemainingOpenStages: proposal.RemainingOpenStages,
		StandingCloseDate:   standingDate(standing),
	}
}

// standingNoDate is a deal holding no close date at all — a real case, the
// `missing` flag exists for it. It cannot collide with a real date, every one of
// which is formatted YYYY-MM-DD.
const standingNoDate = "none"

func standingDate(current *time.Time) string {
	if current == nil {
		return standingNoDate
	}
	return current.Format(time.DateOnly)
}

// SameQuestionAs reports whether an earlier refused correction already answered
// this one.
//
// Neither date can carry this. The proposed date is today plus a stage-velocity
// offset, so it moves every calendar day; the standing date moves with it,
// because the sweep writes its guess onto the deal before staging. A memory
// keyed on either recognises a refusal for exactly one night, which is
// indistinguishable from no memory at all on every night after the first.
//
// What holds still is the JUDGMENT the rep turned down: this deal has N stages
// left, so push it out by a stage-worth of the usual pace for each. Refusing
// that is refusing the reasoning, and the reasoning is the same tomorrow. It
// stops being the same when the deal advances a stage — then N drops, the guess
// is drawn from a genuinely different situation, and the rep is asked again.
//
// The second term is what keeps a rep's own date askable. A refusal is
// remembered with no expiry, so without it one "no" would end close-date
// hygiene on that deal for good: the rep refuses a guess, puts their own date
// on the deal, that one slips in turn, and nobody ever tells them.
//
// It asks whether the deal is still sitting where that refusal left it. The
// sweep writes its proposal onto the deal before staging, so a deal nobody has
// touched since stands at exactly the date the refused card offered — the
// refusal still describes it. A deal standing anywhere else has been moved by a
// person, and what they chose has not been asked about yet.
//
// Note which two values that compares: TONIGHT's standing date against the
// EARLIER proposal. Comparing two standing dates, or two proposed ones, is the
// trap this went round twice — both move with the calendar, so the memory would
// hold for exactly one night.
func (p RefusalProbe) SameQuestionAs(earlier CloseDateCorrection) bool {
	if p.RemainingOpenStages == "" || earlier.RemainingOpenStages == "" {
		// A payload staged before this key existed carries no stage count, and
		// two unknowns are not a match: reading them as one would let the oldest
		// refusal in the queue silence every deal that ever reaches it.
		return false
	}
	return p.RemainingOpenStages == earlier.RemainingOpenStages &&
		p.StandingCloseDate == earlier.ExpectedCloseDate
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
