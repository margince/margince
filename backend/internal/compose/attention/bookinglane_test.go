// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The booking_health lane: the reader's own paused booking page, withheld for a
// caller with no human behind it.

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubBookingHealth struct {
	rows []BookingConcern
	err  error
}

func (s *stubBookingHealth) BookingConcerns(context.Context) ([]BookingConcern, error) {
	return s.rows, s.err
}

func bookingLaneService(health BookingHealth) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
	).WithBookingHealth(health)
}

func TestAPausedBookingPageIsOneCardNamingItsBlocker(t *testing.T) {
	page := ids.NewV7()
	svc := bookingLaneService(&stubBookingHealth{rows: []BookingConcern{{PageID: page, Blocker: "calendar_read_only"}}})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.BookingHealth == nil || len(*out.BookingHealth) != 1 {
		t.Fatalf("booking_health carries %v, want one card", out.BookingHealth)
	}
	if out.Counts.BookingHealth == nil || *out.Counts.BookingHealth != 1 {
		t.Fatalf("booking_health count = %v, want 1", out.Counts.BookingHealth)
	}
	card := (*out.BookingHealth)[0]
	if want := page.String() + ":calendar_read_only"; card.Source != "booking_health" || card.Id != want {
		t.Errorf("card = %s/%s, want booking_health/%s", card.Source, card.Id, want)
	}
	if card.Kind == nil || *card.Kind != "calendar_read_only" {
		t.Errorf("kind = %v, want the blocker", card.Kind)
	}
	if card.CauseRef == nil || *card.CauseRef != "booking_health:calendar_read_only" {
		t.Errorf("cause_ref = %v, want the blocker as the cause", card.CauseRef)
	}
	if len(card.Actions) != 0 {
		t.Error("a booking card promises no verbs")
	}
}

func TestARefusedBookingHealthReadIsNamedAsWithheld(t *testing.T) {
	svc := bookingLaneService(&stubBookingHealth{err: apperrors.ErrPermissionDenied})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.BookingHealth != nil {
		t.Fatalf("booking_health = %v, want the lane withheld", out.BookingHealth)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("booking_health")) {
		t.Fatal("a refused booking_health read was not named in lanes_omitted")
	}
}

func TestAnUnboundBookingLaneIsAbsent(t *testing.T) {
	out, err := captureLaneService(nil).Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.BookingHealth != nil {
		t.Fatalf("booking_health = %v, want the lane absent", out.BookingHealth)
	}
}
