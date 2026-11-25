// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// dealsHolding answers one deal, and ErrNotFound for any other id.
type dealsHolding struct {
	id   ids.UUID
	deal CoachDeal
}

func (d dealsHolding) DealToCoach(_ context.Context, id ids.UUID) (CoachDeal, error) {
	if id != d.id {
		return CoachDeal{}, apperrors.ErrNotFound
	}
	return d.deal, nil
}

var (
	coachedRep  = ids.From[ids.UserKind](ids.MustParse("01a05500-0000-7000-8000-0000000000a1"))
	coachedDeal = ids.MustParse("01a05500-0000-7000-8000-0000000000d1")
)

func depotRollout(planned bool) dealsHolding {
	return dealsHolding{id: coachedDeal, deal: CoachDeal{
		Name: "Depot rollout", Owner: coachedRep.UUID, Open: true, NextStepPlanned: planned,
		QuietSince: time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC),
	}}
}

func nextStepAsk() CoachAsk {
	return CoachAsk{Recipient: coachedRep, Kind: crmcontracts.NoticeKindCoachDealNeedsNextStep, Deal: coachedDeal}
}

func TestANextStepNoticeNamesItsDealAndWhatIsTrueOfIt(t *testing.T) {
	t.Parallel()
	for planned, want := range map[bool]string{
		false: "Depot rollout has no next step planned, and nothing has happened on it since 2026-11-12.",
		true:  "Depot rollout has a next step planned, but nothing has happened on it since 2026-11-12.",
	} {
		line, err := coachHeadline(context.Background(), depotRollout(planned), nextStepAsk())
		if err != nil {
			t.Fatalf("wording a next-step notice: %v", err)
		}
		if line.text != want {
			t.Errorf("planned=%v: the headline reads %q, want %q", planned, line.text, want)
		}
		if line.target != (Target{Type: "deal", ID: coachedDeal}) {
			t.Errorf("planned=%v: the notice points at %+v, want the deal", planned, line.target)
		}
	}
}

func TestANextStepNoticeNeedsItsDealAndNoOtherKindTakesOne(t *testing.T) {
	t.Parallel()
	noDeal := nextStepAsk()
	noDeal.Deal = ids.Nil
	general := nextStepAsk()
	general.Kind = crmcontracts.NoticeKindCoachGeneral
	for code, ask := range map[string]CoachAsk{"missing_deal": noDeal, "deal_not_for_kind": general} {
		var parse *values.ParseError
		if err := checkCoachDeal(ask); !errors.As(err, &parse) || parse.Code != code || parse.Field != fieldDeal {
			t.Errorf("%s: the ask answered %v", code, err)
		}
	}
	if err := checkCoachDeal(nextStepAsk()); err != nil {
		t.Errorf("a next-step ask with its deal was refused: %v", err)
	}
}

func TestANextStepNoticeRefusesADealItCannotBeAbout(t *testing.T) {
	t.Parallel()
	closed := depotRollout(false)
	closed.deal.Open = false
	someoneElses := depotRollout(false)
	someoneElses.deal.Owner = ids.NewV7()
	for code, deals := range map[string]dealsHolding{"deal_closed": closed, "deal_not_theirs": someoneElses} {
		var parse *values.ParseError
		if _, err := coachHeadline(context.Background(), deals, nextStepAsk()); !errors.As(err, &parse) || parse.Code != code {
			t.Errorf("%s: the headline answered %v", code, err)
		}
	}
	unseen := nextStepAsk()
	unseen.Deal = ids.NewV7()
	if _, err := coachHeadline(context.Background(), depotRollout(false), unseen); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a deal the coach cannot read answered %v, want not found", err)
	}
}

func TestAKindWithNoRecordKeepsItsFixedHeadline(t *testing.T) {
	t.Parallel()
	line, err := coachHeadline(context.Background(), nil,
		CoachAsk{Recipient: coachedRep, Kind: crmcontracts.NoticeKindCoachGeneral})
	if err != nil || line.text != "Your lead left you a note" || line.target.Named() {
		t.Fatalf("a general note came back as %+v, %v", line, err)
	}
}
