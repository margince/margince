// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two lanes report a silence: a deal nobody is moving, and a person nobody is
// talking to. They rest on different records and answer to different readers,
// so each proves separately that it speaks only when it has ground to.

type stubAtRisk struct {
	rows []RiskyDeal
	err  error
}

func (s stubAtRisk) Quiet(context.Context) ([]RiskyDeal, error) { return s.rows, s.err }

type stubDecay struct {
	rows  []QuietRelationship
	err   error
	calls int
}

func (s *stubDecay) Lapsed(context.Context) ([]QuietRelationship, error) {
	s.calls++
	return s.rows, s.err
}

// A quiet deal names the number of days it has been quiet. The whole reason the
// lane asks at a shorter window than the stalled status is to speak sooner, and
// a card that said only "at risk" would hide which patience produced it.
func TestAQuietDealSaysHowLongItHasBeenQuiet(t *testing.T) {
	deal := ids.NewV7()
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil,
		stubAtRisk{rows: []RiskyDeal{{DealID: deal, Name: "Fleet retrofit", QuietDays: 19}}}, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AtRisk == nil {
		t.Fatal("the at-risk lane is absent, want one deal")
	}
	items := *out.AtRisk
	if len(items) != 1 {
		t.Fatalf("the lane carries %d items, want 1", len(items))
	}
	if items[0].Title == nil || *items[0].Title != "Fleet retrofit" {
		t.Errorf("title = %v, want the deal name", items[0].Title)
	}
	if items[0].Detail == nil || *items[0].Detail != "19" {
		t.Errorf("detail = %s, want the idle day count", stringOr(items[0].Detail))
	}
	if items[0].Kind == nil || *items[0].Kind != "quiet" {
		t.Errorf("kind = %s, want the ground it was admitted on", stringOr(items[0].Kind))
	}
	if items[0].Subject == nil || items[0].Subject.Type != "deal" {
		t.Errorf("subject = %v, want the deal", items[0].Subject)
	}
}

// A deal past its close date is reported as overdue rather than merely quiet.
// The two grounds read differently to a rep: a date the customer agreed to has
// passed, which is a harder fact than a silence nobody agreed to.
func TestADealPastItsCloseDateReportsThatGroundRatherThanSilence(t *testing.T) {
	closed := readInstant.AddDate(0, 0, -30)
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil,
		stubAtRisk{rows: []RiskyDeal{{
			DealID: ids.NewV7(), Name: "Closing last month",
			QuietDays: 2, CloseOverdue: true, ExpectedCloseDate: &closed,
		}}}, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	item := (*out.AtRisk)[0]
	if item.Kind == nil || *item.Kind != "close_overdue" {
		t.Errorf("kind = %s, want close_overdue", stringOr(item.Kind))
	}
	if item.Overdue == nil || !*item.Overdue {
		t.Errorf("overdue = %v, want true", item.Overdue)
	}
	if item.DueAt == nil || !item.DueAt.Equal(closed) {
		t.Errorf("due_at = %v, want the expected close date", item.DueAt)
	}
}

// A withheld risk lane is NAMED, not reported empty. "Nothing is at risk" is a
// claim about the pipeline; "you may not read the deals" is a claim about the
// reader, and only one of them is true here.
func TestAWithheldRiskLaneIsNamedRatherThanReportedEmpty(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil,
		stubAtRisk{err: apperrors.ErrPermissionDenied}, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AtRisk != nil {
		t.Errorf("a withheld lane sent %v, want no lane at all", *out.AtRisk)
	}
	var named bool
	for _, lane := range *out.LanesOmitted {
		if lane == "at_risk" {
			named = true
		}
	}
	if !named {
		t.Errorf("lanes_omitted = %v, want it to name at_risk", *out.LanesOmitted)
	}
}

// A feed with no risk reader sends no lane at all, the same absent-not-empty
// rule the commitments lane keeps.
func TestAFeedWithNoRiskReaderSendsNoRiskLane(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AtRisk != nil {
		t.Errorf("at_risk = %v, want the lane absent", *out.AtRisk)
	}
	if out.Counts.AtRisk != nil {
		t.Errorf("counts.at_risk = %v, want it absent too", out.Counts.AtRisk)
	}
}

// A lapsed relationship carries the span, the person it is about, and when
// they last spoke. All three, because the card is read as a chronology: the
// number is what a rep acts on, the name is who to act on, and the date is
// what makes the claim checkable against that contact's own timeline.
func TestALapsedRelationshipCarriesItsSpanAndItsLastExchange(t *testing.T) {
	person := ids.NewV7()
	spoke := readInstant.AddDate(0, 0, -63)
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil,
		&stubDecay{rows: []QuietRelationship{
			{PersonID: person, Name: "Dana Weiss", QuietDays: 63, LastAt: spoke},
		}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.RelationshipDecay == nil {
		t.Fatal("the decay lane is absent, want one lapsed relationship")
	}
	item := (*out.RelationshipDecay)[0]
	if item.Title == nil || *item.Title != "Dana Weiss" {
		t.Errorf("title = %s, want the contact's name", stringOr(item.Title))
	}
	if item.Detail == nil || *item.Detail != "63" {
		t.Errorf("detail = %s, want the derived silence in days", stringOr(item.Detail))
	}
	if item.OccurredAt == nil || !item.OccurredAt.Equal(spoke) {
		t.Errorf("occurred_at = %v, want the last exchange %v", item.OccurredAt, spoke)
	}
	// The subject is the PERSON, not the edge: the card's one move is opening
	// that contact, and a subject naming anything else sends the reader
	// somewhere they cannot act.
	if item.Subject == nil || item.Subject.Type != "person" {
		t.Errorf("subject = %v, want the person the silence is about", item.Subject)
	}
	// No verb, exactly as the risk card offers none. What to do about a lapsed
	// relationship is a judgement about that person; a lane answering it here
	// would be deciding rather than warning.
	if len(item.Actions) != 0 {
		t.Errorf("actions = %v, want none — the lane warns, it does not decide", item.Actions)
	}
	if out.Counts.RelationshipDecay == nil || *out.Counts.RelationshipDecay != 1 {
		t.Errorf("counts.relationship_decay = %v, want 1", out.Counts.RelationshipDecay)
	}
}

// A withheld decay lane is NAMED. "You are in touch with everyone" and "you
// may not read these relationships" are different answers, and reporting the
// second as the first tells a rep their day is clear when it is not.
func TestAWithheldDecayLaneIsNamedRatherThanReportedEmpty(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil,
		&stubDecay{err: apperrors.ErrPermissionDenied},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("a refused lane must not fail the read: %v", err)
	}
	if out.RelationshipDecay != nil {
		t.Errorf("a withheld lane sent %v, want no lane", *out.RelationshipDecay)
	}
	var named bool
	for _, lane := range *out.LanesOmitted {
		if lane == "relationship_decay" {
			named = true
		}
	}
	if !named {
		t.Errorf("lanes_omitted = %v, want it to name relationship_decay", *out.LanesOmitted)
	}
}

// A feed with no decay reader sends no lane at all, the same absent-not-empty
// rule the other optional lanes keep.
func TestAFeedWithNoDecayReaderSendsNoDecayLane(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.RelationshipDecay != nil {
		t.Errorf("relationship_decay = %v, want the lane absent", *out.RelationshipDecay)
	}
	if out.Counts.RelationshipDecay != nil {
		t.Errorf("counts.relationship_decay = %v, want it absent too", out.Counts.RelationshipDecay)
	}
}
