// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contentGateWeek is a week where every count has one row the rep may read and
// one limited to a colleague. A count that skips the content gate reads one
// too many.
type contentGateWeek struct {
	e          *weekEnv
	owner      *pgx.Conn
	start, end time.Time
	// The readable meeting's start: the earliest meeting the rep may read.
	readableMeetingAt time.Time
	// The readable task's creation: the earliest task the rep may read.
	readableTaskAt time.Time
}

func seedContentGateWeek(t *testing.T) *contentGateWeek {
	t.Helper()
	e := setupWeekly(t)
	w := &contentGateWeek{e: e, owner: integration.OwnerConn(t)}
	// The deals are minted now and must exist when the week closes, so the
	// week is the one around now rather than weekClock's.
	w.end = clocktest.Now(t).Add(time.Hour)
	w.start = w.end.AddDate(0, 0, -7)
	w.readableMeetingAt = w.end.AddDate(0, 0, -3)
	hiddenMeetingAt := w.end.AddDate(0, 0, -4)

	readableMeeting := w.meeting(t, "Readable meeting", w.readableMeetingAt)
	hiddenMeeting := w.meeting(t, "Limited meeting", hiddenMeetingAt)
	w.limit(t, hiddenMeeting)
	// The readable meeting's only next step is limited. The limited meeting's
	// next step inherits its limit, and it is the rep's earliest task, so the
	// task start must skip it.
	w.limit(t, w.task(t, "Limited next step", w.readableMeetingAt.Add(time.Hour), &readableMeeting))
	w.task(t, "Next step of a limited meeting", hiddenMeetingAt.Add(time.Hour), &hiddenMeeting)

	pipeline, stage, _ := integration.DealFixture(t, e.Env)
	readableDeal := e.SeedDeal(t, "Readable work", pipeline, stage, &e.Rep1)
	hiddenDeal := e.SeedDeal(t, "Limited work", pipeline, stage, &e.Rep1)
	w.readableTaskAt = w.end.AddDate(0, 0, -1)
	w.linkDeal(t, w.task(t, "Readable deal task", w.readableTaskAt, nil), readableDeal)
	limitedDealTask := w.task(t, "Limited deal task", w.end.AddDate(0, 0, -2), nil)
	w.limit(t, limitedDealTask)
	w.linkDeal(t, limitedDealTask, hiddenDeal)

	// Two contacts on each deal. One of the limited deal's touches is limited,
	// so the rep sees one stakeholder there and two on the readable deal.
	touchAt := w.end.AddDate(0, 0, -2)
	for _, deal := range []ids.UUID{readableDeal, hiddenDeal} {
		for i, name := range []string{"First stakeholder", "Second stakeholder"} {
			touch := integration.SeedIDRow(t, w.owner, `
				INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
				VALUES ($1, 'note', $2, $3, 'manual', 'human:' || $4::text)`,
				name, touchAt, e.Rep3)
			contact := integration.SeedIDRow(t, w.owner,
				`INSERT INTO contact (id, full_name, source, captured_by)
				 VALUES ($1, $2, 'manual', 'human:x')`, name)
			linkToContact(t, w.owner, touch, contact)
			w.linkDeal(t, touch, deal)
			if deal == hiddenDeal && i == 1 {
				w.limit(t, touch)
			}
		}
	}
	return w
}

// meeting seeds a held meeting the rep hosted and a colleague filed, so the
// rep is credited with it without being its author.
func (w *contentGateWeek) meeting(t *testing.T, subject string, at time.Time) ids.UUID {
	t.Helper()
	return integration.SeedIDRow(t, w.owner, `
		INSERT INTO activity (id, kind, subject, occurred_at, meeting_status,
		                      host_user_id, source, captured_by)
		VALUES ($1, 'meeting', $2, $3, 'held', $4, 'manual', 'human:' || $5::text)`,
		subject, at, w.e.Rep1, w.e.Rep3)
}

// task seeds an open task for the rep that a colleague filed, raised from
// meeting when one is given.
func (w *contentGateWeek) task(t *testing.T, subject string, created time.Time, meeting *ids.UUID) ids.UUID {
	t.Helper()
	return integration.SeedIDRow(t, w.owner, `
		INSERT INTO activity (id, kind, subject, occurred_at, created_at, source_activity_id,
		                      assignee_id, source, captured_by)
		VALUES ($1, 'task', $2, $3, $3, $4, $5, 'manual', 'human:' || $6::text)`,
		subject, created, meeting, w.e.Rep1, w.e.Rep3)
}

// limit hands the activity's content to Rep2 alone, through the real writer
// and as its author.
func (w *contentGateWeek) limit(t *testing.T, activity ids.UUID) {
	t.Helper()
	author := w.e.As(w.e.Rep3, []ids.UUID{w.e.Team1}, integration.AdminPerms)
	if _, err := w.e.Activities.SetAudience(author, ids.From[ids.ActivityKind](activity), activities.SetAudienceInput{
		Audience: "selected",
		Members:  []activities.AudienceMember{{SubjectType: "user", SubjectID: w.e.Rep2}},
	}); err != nil {
		t.Fatalf("limiting the activity to a colleague: %v", err)
	}
}

func (w *contentGateWeek) linkDeal(t *testing.T, activity, deal ids.UUID) {
	t.Helper()
	if _, err := w.owner.Exec(context.Background(), `
		INSERT INTO activity_link (activity_id, entity_type, deal_id)
		VALUES ($1, 'deal', $2)`, activity, deal); err != nil {
		t.Fatalf("linking the activity to its deal: %v", err)
	}
}

// The week's meeting counts, deal block and coverage starts count only the
// activity the rep may read. Each of them reads several sets of activity under
// one content gate, and each set holds one row limited to a colleague.
func TestTheWeekCountsOnlyActivityTheRepMayRead(t *testing.T) {
	w := seedContentGateWeek(t)
	e := w.e
	err := e.DB().Tx(e.repCtx, func(tx pgx.Tx) error {
		held, withNextStep, err := countWeekMeetings(e.repCtx, tx, e.Rep1, w.start, w.end)
		if err != nil {
			return err
		}
		if held != 1 || withNextStep != 0 {
			t.Errorf("held %d with a next step %d, want 1 and 0: the limited meeting and the limited next step are not the rep's to read",
				held, withNextStep)
		}
		block, err := scoreDeals(e.repCtx, tx, e.Rep1, w.start, w.end)
		if err != nil {
			return err
		}
		if block == nil || block.Open != 2 || block.WithNextStep != 1 || block.MultiThreaded != 1 {
			t.Errorf("deal block %+v, want 2 open, 1 with a next step and 1 multi-threaded", block)
		}
		starts, err := readFigureStarts(e.repCtx, tx, []ids.UUID{e.Rep1}, w.end)
		if err != nil {
			return err
		}
		if starts.meetings == nil || !starts.meetings.Equal(w.readableMeetingAt) {
			t.Errorf("meetings start %v, want the readable meeting's %v", starts.meetings, w.readableMeetingAt)
		}
		if starts.tasks == nil || !starts.tasks.Equal(w.readableTaskAt) {
			t.Errorf("tasks start %v, want the readable task's %v", starts.tasks, w.readableTaskAt)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
