// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two OPTIONAL lanes: the promises the rep made, and today's meetings. Each is
// bound only when its reader exists, so each carries a case the required lanes
// do not — whether the installation sends the lane at all.
//
// The other two optional lanes, at-risk and decay, sit in quietlanes_test.go
// because both rest on a record having gone quiet.
//
// They share the feed's stubs and fixed clock from feed_test.go, which is the
// same package. This file exists because that one reached the length ceiling.

type stubCommitments struct {
	rows []Commitment
	err  error
	// calls counts the reads, so a lane assembled twice is caught here rather
	// than by a reader noticing their withheld lane named twice.
	calls int
	// by records the instant the lane asked for, so a test can prove the
	// window rather than assume it.
	by *time.Time
}

func (s *stubCommitments) DueBy(_ context.Context, by time.Time, _ int) ([]Commitment, error) {
	s.by = &by
	s.calls++
	return s.rows, s.err
}

func promise(body string, due time.Time) Commitment {
	return Commitment{
		ID:          ids.NewV7(),
		PersonID:    ids.NewV7(),
		Body:        body,
		Quote:       "Ich schicke Ihnen die Referenzliste bis Dienstag.",
		SourceLabel: "Rückfragen zum Angebot",
		OccurredAt:  readInstant.Add(-7 * 24 * time.Hour),
		DueAt:       due,
	}
}

// A commitment lane exists at all, and it carries the promise's own words with
// the evidence beside them. A card that showed only the paraphrase would ask a
// reader to trust the extractor, which is what the claim contract's
// source_quote rule exists to prevent.
func TestACommitmentCarriesThePromiseAndTheWordsItWasReadFrom(t *testing.T) {
	due := readInstant.Add(2 * time.Hour)
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{rows: []Commitment{promise("Referenzliste an Herrn Vogt schicken", due)}}, nil, nil,
		nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Commitments == nil {
		t.Fatal("the commitments lane is absent, want one item")
	}
	items := *out.Commitments
	if len(items) != 1 {
		t.Fatalf("the commitments lane carries %d items, want 1", len(items))
	}
	item := items[0]
	if item.Title == nil || *item.Title != "Referenzliste an Herrn Vogt schicken" {
		t.Errorf("title = %v, want the promise body", item.Title)
	}
	if item.Detail == nil || *item.Detail != "Ich schicke Ihnen die Referenzliste bis Dienstag." {
		t.Errorf("detail = %v, want the verbatim quote", item.Detail)
	}
	if item.Subject == nil || item.Subject.Type != "person" {
		t.Errorf("subject = %v, want the person it was promised to", item.Subject)
	}
	if item.Source != "conversation_claim" {
		t.Errorf("source = %q, want conversation_claim", item.Source)
	}
	if out.Counts.Commitments == nil || *out.Counts.Commitments != 1 {
		t.Errorf("counts.commitments = %v, want 1", out.Counts.Commitments)
	}
}

// A promise already past its due date says so on the wire. The reader must not
// have to compare two timestamps themselves, and every surface has to agree on
// where the line falls — which is why the server resolves it.
func TestAnOverduePromiseSaysSoRatherThanLeavingTheReaderToCompareDates(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{rows: []Commitment{
			promise("Angebot nachfassen", readInstant.Add(-48*time.Hour)),
			promise("Termin bestätigen", readInstant.Add(3*time.Hour)),
		}}, nil, nil,
		nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	items := *out.Commitments
	if len(items) != 2 {
		t.Fatalf("the lane carries %d items, want 2", len(items))
	}
	if items[0].Overdue == nil || !*items[0].Overdue {
		t.Errorf("the promise due two days ago reports overdue = %v, want true", overdueOf(items[0]))
	}
	if items[1].Overdue == nil || *items[1].Overdue {
		t.Errorf("the promise due later today reports overdue = %v, want false", overdueOf(items[1]))
	}
}

// The lane stops at the SAME instant the planned lane does. Two due-dated lanes
// on one screen that disagreed about when today ends would put a promise and a
// task falling on the same afternoon on opposite sides of the boundary.
func TestBothDueDatedLanesStopAtTheSameEndOfDay(t *testing.T) {
	commitments := &stubCommitments{}
	tasks := &stubTasks{}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{}, stubBriefing{},
		commitments, nil, nil, nil, nil, nil, nil, fixedClock)
	if _, err := svc.Assemble(context.Background()); err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if commitments.by == nil || tasks.until == nil {
		t.Fatal("one of the two due-dated lanes was never asked for a window")
	}
	if !commitments.by.Equal(*tasks.until) {
		t.Errorf("commitments stop at %s and tasks at %s, want the same instant",
			commitments.by, tasks.until)
	}
}

// A reader who may not see the claims is TOLD the lane was withheld. Reporting
// it empty would say "you owe nobody anything", which is a different and false
// statement about their day.
func TestAWithheldCommitmentLaneIsNamedRatherThanReportedEmpty(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{err: apperrors.ErrPermissionDenied}, nil, nil,
		nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Commitments != nil {
		t.Errorf("a withheld lane sent %v, want no lane at all", *out.Commitments)
	}
	if out.LanesOmitted == nil {
		t.Fatal("no lane was named as omitted, want commitments")
	}
	var named bool
	for _, lane := range *out.LanesOmitted {
		if lane == "commitments" {
			named = true
		}
	}
	if !named {
		t.Errorf("lanes_omitted = %v, want it to name commitments", *out.LanesOmitted)
	}
}

// An installation that binds no claim reader sends NO lane, rather than an
// empty one. The two are different facts: "this feed does not do commitments"
// against "you have none today".
func TestAFeedWithNoClaimReaderSendsNoCommitmentLaneAtAll(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Commitments != nil {
		t.Errorf("commitments = %v, want the lane absent", *out.Commitments)
	}
	if out.Counts.Commitments != nil {
		t.Errorf("counts.commitments = %v, want it absent too", out.Counts.Commitments)
	}
}

// A broken claim read is an ERROR, never a quiet lane. A feed that swallowed it
// would draw a clear day over a read that failed.
func TestABrokenCommitmentReadFailsTheFeedRatherThanReadingAsAClearDay(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{err: errors.New("the claim read fell over")}, nil, nil,
		nil, nil, nil, nil,
		fixedClock)
	if _, err := svc.Assemble(context.Background()); err == nil {
		t.Fatal("a failed commitment read assembled a day, want the error surfaced")
	}
}

// overdueOf reads the flag for a failure message. Printing the pointer itself
// reports an address, which tells a reader running the test nothing about what
// the wire actually said.
func overdueOf(item crmcontracts.AttentionItem) string {
	if item.Overdue == nil {
		return "absent"
	}
	return strconv.FormatBool(*item.Overdue)
}

// stringOr reads a wire string for a failure message. Printing the pointer
// reports an address, which tells a reader running the test nothing.
func stringOr(v *string) string {
	if v == nil {
		return "absent"
	}
	return *v
}

type stubMeetings struct {
	rows  []Meeting
	err   error
	calls int
	// from records the window's start, so a test can prove the lane asks from
	// NOW rather than from the start of the day.
	from *time.Time
}

func (s *stubMeetings) Today(_ context.Context, from, _ time.Time, _ int) ([]Meeting, error) {
	s.from = &from
	s.calls++
	return s.rows, s.err
}

// The lane asks from the READ INSTANT, not from midnight. A meeting that ended
// an hour ago cannot be prepared for, and listing it under a lane of what is
// still ahead would be plainly false.
func TestTheMeetingLaneAsksFromNowRatherThanFromTheStartOfTheDay(t *testing.T) {
	meetings := &stubMeetings{}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		nil, nil, nil, meetings, nil, nil, nil, fixedClock,
	)
	if _, err := svc.Assemble(context.Background()); err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if meetings.from == nil {
		t.Fatal("the meeting lane was never asked for a window")
	}
	if !meetings.from.Equal(readInstant) {
		t.Errorf("the lane asks from %s, want the read instant %s", meetings.from, readInstant)
	}
}

// A meeting carries its own subject and the instant it starts. The subject is
// what a rep recognises and the start is what they are racing.
func TestAMeetingCarriesItsSubjectAndWhenItStarts(t *testing.T) {
	starts := readInstant.Add(90 * time.Minute)
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil,
		&stubMeetings{rows: []Meeting{{ID: ids.NewV7(), Subject: "Vogt — Angebotsbesprechung", StartsAt: starts}}}, nil, nil, nil,
		fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Meetings == nil {
		t.Fatal("the meeting lane is absent, want one meeting")
	}
	item := (*out.Meetings)[0]
	if item.Title == nil || *item.Title != "Vogt — Angebotsbesprechung" {
		t.Errorf("title = %s, want the meeting subject", stringOr(item.Title))
	}
	if item.DueAt == nil || !item.DueAt.Equal(starts) {
		t.Errorf("due_at = %v, want when the meeting starts", item.DueAt)
	}
	// A meeting that has not started cannot be late, so the flag stays off.
	if item.Overdue != nil && *item.Overdue {
		t.Error("a meeting still ahead reports overdue, want not")
	}
	if out.Counts.Meetings == nil || *out.Counts.Meetings != 1 {
		t.Errorf("counts.meetings = %v, want 1", out.Counts.Meetings)
	}
}

// A withheld meeting lane is NAMED. "No meetings today" and "you may not read
// the calendar" are different answers and only one of them is true.
func TestAWithheldMeetingLaneIsNamedRatherThanReportedEmpty(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil,
		&stubMeetings{err: apperrors.ErrPermissionDenied}, nil, nil, nil, fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Meetings != nil {
		t.Errorf("a withheld lane sent %v, want no lane", *out.Meetings)
	}
	var named bool
	for _, lane := range *out.LanesOmitted {
		if lane == "meetings" {
			named = true
		}
	}
	if !named {
		t.Errorf("lanes_omitted = %v, want it to name meetings", *out.LanesOmitted)
	}
}
