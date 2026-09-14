// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestPrivacyUrgencyUsesPreparationWindow(t *testing.T) {
	for _, days := range []int{-1, 0, 7, 8, 23} {
		due := rankInstant.Add(time.Duration(days) * 24 * time.Hour)
		row := classifyLegalDeadline(item("case", "notice_case", withDue(due)), rankInstant)
		if urgentWork(row) != (days <= 7) {
			t.Errorf("deadline in %d days: urgent=%v", days, urgentWork(row))
		}
		if row.item.DueAt == nil || !row.item.DueAt.Equal(due) {
			t.Fatalf("deadline was changed: %+v", row.item)
		}
	}
}

func TestMeetingPreparationClaimsRequireKnownEvidence(t *testing.T) {
	for _, tc := range []struct {
		known, needs bool
		kind         string
		consequence  crmcontracts.WorklistItemConsequence
	}{
		{false, false, "", "none"}, {true, false, "prepared", "none"}, {true, true, "unprepared", "meeting_unprepared"},
	} {
		row := classifyMeeting(meetingItem(Meeting{PrepKnown: tc.known, NeedsPrep: tc.needs, StartsAt: rankInstant.Add(time.Hour)}), rankInstant).item
		if row.Consequence != tc.consequence {
			t.Errorf("known=%v needs=%v: consequence=%s", tc.known, tc.needs, row.Consequence)
		}
		if tc.kind == "" && row.Kind != nil {
			t.Error("unknown preparation was labelled known")
		}
		if tc.kind != "" && (row.Kind == nil || *row.Kind != tc.kind) {
			t.Errorf("missing kind %s", tc.kind)
		}
	}
}

type planWorkStub struct {
	entries []PlanWork
	err     error
}

func (p planWorkStub) DuePlan(context.Context, ids.UUID, time.Time) ([]PlanWork, error) {
	return p.entries, p.err
}

func TestDuePlanJoinsTheAgendaAndItsCounts(t *testing.T) {
	ctx := meetingPrepReader()
	owner := readerOf(ctx)
	svc := meetingPrepService(nil).WithWeeklyPlans(planWorkStub{entries: []PlanWork{{ID: ids.NewV7(), OwnerID: owner, Label: "Send the proposal", DueAt: rankInstant.Add(-time.Hour)}}})
	day, err := svc.Worklist(ctx, "mine", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Queue) != 1 || day.Queue[0].Source != sourceWeeklyCommitment {
		t.Fatalf("plan absent from agenda: %+v", day.Queue)
	}
	row := day.Queue[0]
	if day.Summary.Urgent != 1 || row.Urgent == nil || !*row.Urgent {
		t.Fatalf("urgent facts disagree: %+v", day)
	}
	if row.Owner == nil || row.Owner.Id == nil || ids.UUID(*row.Owner.Id) != owner {
		t.Fatalf("plan owner missing: %+v", row.Owner)
	}
	if got := countFor(t, day, "tasks"); got.Considered != 1 {
		t.Fatalf("count=%+v", got)
	}
	if len(svc.planRows) != 0 {
		t.Fatal("request state escaped into shared service")
	}
}

func TestFailedPlanReadCannotLookLikeAnEmptyDay(t *testing.T) {
	svc := meetingPrepService(nil).WithWeeklyPlans(planWorkStub{err: errors.New("plan read failed")})
	day, err := svc.Worklist(meetingPrepReader(), "mine", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range day.SourcesUnavailable {
		if missing.Source == sourceWeeklyCommitment && missing.Reason == "failed" && missing.Category != nil && *missing.Category == "tasks" {
			return
		}
	}
	t.Fatalf("missing source not reported: %+v", day.SourcesUnavailable)
}
