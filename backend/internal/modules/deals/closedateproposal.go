// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a close-date correction IS, and the seams the composition root fills to
// raise one: the staged payload a human weighs, the kind it is staged under, and
// the readers that let the nightly sweep ask the approvals engine what it has
// already proposed and what a human has already refused.
//
// Apart from the sweep that drives it because the shape outlives any one pass —
// the confirm effect unmarshals this payload, the queue renders it, and the
// staging identity is drawn from its fields. The sweep is one caller.

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
	// HasRefusedCorrection reports whether a human has already turned down a
	// correction on this deal.
	//
	// The staging path remembers a refusal against the date it was about, which
	// is what keeps a genuinely NEW question askable. One branch of this sweep
	// needs the coarser answer: the keep-alive pass asks a rep to confirm the
	// provisional date the sweep itself wrote, and after a refusal that card
	// would return every night under a fresh identity — the date it names is the
	// one the rep just declined to accept. A deal whose rep has said no is not
	// kept alive; it waits for the deal to move.
	HasRefusedCorrection(ctx context.Context, dealID ids.UUID) (bool, error)
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
	// StandingCloseDate is PreviousCloseDate in the form a staging identity can
	// be keyed on: never absent, and "none" for a deal holding no date at all.
	//
	// It exists beside the nullable field rather than replacing it because a
	// staging identity must be a string the payload carries with the same value
	// (canonicalIdentity enforces both), and an absent key never matches. The
	// card renders the nullable one, which reads as "no previous date" rather
	// than as the word none.
	StandingCloseDate string `json:"standing_close_date"`
}

// StandingCloseDate spells the date a deal currently holds for an identity.
//
// A deal with no date at all is a real case — the `missing` flag exists for it —
// and it must be one value rather than an absent key, or the proposals raised
// about it carry no rejection memory at all.
func StandingCloseDate(current *string) string {
	if current == nil {
		return "none"
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
