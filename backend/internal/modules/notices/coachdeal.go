// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/targettype"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// fieldDeal is the contract's spelling of the deal a coaching notice is about.
const fieldDeal = "deal_id"

// CoachAsk is one coaching notice as the coach asked for it.
type CoachAsk struct {
	Recipient ids.UserID
	Kind      crmcontracts.NoticeKind
	Note      string
	// Deal is the deal the notice is about: required on the next-step kind,
	// refused on every other.
	Deal ids.UUID
}

// CoachReach is what compose binds for the coaching path: whom the caller
// may coach, and what they may read of the deal they name.
type CoachReach struct {
	Mates Teammates
	Deals CoachDeals
}

// CoachDeals reads one deal as the calling coach may see it. A deal the coach
// cannot read answers apperrors.ErrNotFound, as GET /deals/{id} does.
type CoachDeals interface {
	DealToCoach(ctx context.Context, deal ids.UUID) (CoachDeal, error)
}

// CoachDeal is what a next-step notice says about its deal.
type CoachDeal struct {
	Name            string
	Owner           ids.UUID
	Open            bool
	NextStepPlanned bool
	// QuietSince is the deal's last activity, else the day it was opened.
	QuietSince time.Time
}

// coachLine is a coaching notice's headline: the sentence it is stored as,
// that sentence in the stored language, and the record it is about.
type coachLine struct {
	sentence *systemtext.Component
	text     string
	target   Target
}

// checkCoachDeal holds the request's shape. The next-step kind names its deal,
// and no other kind names one, since its headline would ignore it.
func checkCoachDeal(ask CoachAsk) error {
	wantsDeal := ask.Kind == crmcontracts.NoticeKindCoachDealNeedsNextStep
	switch {
	case wantsDeal && ask.Deal.IsZero():
		return &values.ParseError{Field: fieldDeal, Code: "missing_deal",
			Message: "a next-step notice names the deal it is about; send deal_id"}
	case !wantsDeal && !ask.Deal.IsZero():
		return &values.ParseError{Field: fieldDeal, Code: "deal_not_for_kind",
			Message: "only a next-step notice names a deal; leave deal_id out for this kind"}
	}
	return nil
}

// coachHeadline words the notice. A kind with no record keeps its fixed
// headline; the next-step kind names its deal and what is true of it today.
func coachHeadline(ctx context.Context, deals CoachDeals, ask CoachAsk) (coachLine, error) {
	if ask.Deal.IsZero() {
		code := string(ask.Kind)
		return coachLine{sentence: systemtext.Sentence(code), text: systemtext.Literal(code)}, nil
	}
	deal, err := deals.DealToCoach(ctx, ask.Deal)
	if err != nil {
		return coachLine{}, err
	}
	if !deal.Open {
		return coachLine{}, &values.ParseError{Field: fieldDeal, Code: "deal_closed",
			Message: "the deal is closed, so it has no next step to plan; pick an open deal"}
	}
	if deal.Owner != ask.Recipient.UUID {
		return coachLine{}, &values.ParseError{Field: fieldDeal, Code: "deal_not_theirs",
			Message: "the deal belongs to someone else; coach its owner, or pick one of the recipient's deals"}
	}
	code := "coach.why.next_step_stalled"
	if !deal.NextStepPlanned {
		code = "coach.why.no_next_step"
	}
	sentence := systemtext.Sentence(code,
		systemtext.Text(deal.Name), systemtext.Date(deal.QuietSince.UTC().Format(time.DateOnly)))
	// Stored in English like the fixed headlines; each reader sees their own
	// language through the system text.
	text, err := sentence.Render(textlang.English)
	if err != nil {
		return coachLine{}, err
	}
	return coachLine{sentence: sentence, text: text, target: Target{Type: string(targettype.Deal), ID: ask.Deal}}, nil
}
