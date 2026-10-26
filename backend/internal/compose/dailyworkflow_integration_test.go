// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A day as two reps and their manager read it. The records are written by the
// native writers: tasks through the activity store, mail through the capture
// sink and the reply classifier. The clock is the feed's own, and advancing it
// is how a day passes.

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type dayReaders struct {
	rep1, rep2, manager context.Context
}

// dayNow is a weekday noon in the installation's zone, so a task due in two
// hours is due today on any machine.
var dayNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// day gives the reps the shipped rep role and Rep3 the shipped manager role,
// with Rep3 leading the reps' team. Each reader acts with the authority the
// role resolves to.
func (o *owedEnv) day(t *testing.T) dayReaders {
	t.Helper()
	o.e.GrantRole(t, o.e.Rep1, "rep")
	o.e.GrantRole(t, o.e.Rep2, "rep")
	o.e.GrantRole(t, o.e.Rep3, "manager")
	o.e.WsExec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, o.e.Team1, o.e.Rep3)
	return dayReaders{rep1: o.seated(t, o.e.Rep1), rep2: o.seated(t, o.e.Rep2), manager: o.seated(t, o.e.Rep3)}
}

func (o *owedEnv) seated(t *testing.T, user ids.UUID) context.Context {
	t.Helper()
	authority, _, err := identity.NewService(o.e.Pool).EffectiveAuthority(o.e.Admin(), o.e.WS, user)
	if err != nil {
		t.Fatal(err)
	}
	return o.e.As(user, authority.TeamIDs, authority.Permissions)
}

// page reads the reader's Worklist at the clock's instant.
func (o *owedEnv) page(t *testing.T, reader context.Context, scope string, now time.Time) crmcontracts.Worklist {
	t.Helper()
	feed := newAttentionService(o.e.Pool, approvals.NewService(o.e.DB()), func() time.Time { return now })
	page, err := feed.Worklist(reader, scope, "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading a %s Worklist: %v", scope, err)
	}
	return page
}

func rowIDs(page crmcontracts.Worklist) []string {
	out := make([]string, 0, len(page.Queue))
	for _, row := range page.Queue {
		out = append(out, row.Id)
	}
	slices.Sort(out)
	return out
}

func rowOf(page crmcontracts.Worklist, id string) (crmcontracts.WorklistItem, bool) {
	for _, row := range page.Queue {
		if row.Id == id {
			return row, true
		}
	}
	return crmcontracts.WorklistItem{}, false
}

// taskFor is a task the rep logs for themselves, with their own authority.
func (o *owedEnv) taskFor(t *testing.T, rep context.Context, subject string, due time.Time) string {
	t.Helper()
	actor, _ := principal.Actor(rep)
	user := ids.From[ids.UserKind](actor.UserID)
	task, _, err := o.e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", AssigneeID: &user, DueAt: &due,
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	return task.Id.String()
}

// Each rep's own day holds their own work and nobody else's. The manager's
// team day holds both.
func TestEachReaderSeesTheWorkThatIsTheirs(t *testing.T) {
	o := setupOwed(t)
	readers := o.day(t)
	first := o.taskFor(t, readers.rep1, "Send the pricing sheet", dayNow.Add(2*time.Hour))
	second := o.taskFor(t, readers.rep2, "Book the onsite visit", dayNow.Add(3*time.Hour))

	if got := rowIDs(o.page(t, readers.rep1, "mine", dayNow)); !slices.Equal(got, []string{first}) {
		t.Errorf("rep 1's day holds %v, want only their task %s", got, first)
	}
	if got := rowIDs(o.page(t, readers.rep2, "mine", dayNow)); !slices.Equal(got, []string{second}) {
		t.Errorf("rep 2's day holds %v, want only their task %s", got, second)
	}
	team := rowIDs(o.page(t, readers.manager, "team", dayNow))
	want := []string{first, second}
	slices.Sort(want)
	if !slices.Equal(team, want) {
		t.Errorf("the manager's team day holds %v, want both reps' tasks %v", team, want)
	}
}

// A day passes on the clock alone. The same task moves from today to overdue,
// and no new row appears.
func TestADayPassingMovesTheSameTaskToOverdue(t *testing.T) {
	o := setupOwed(t)
	readers := o.day(t)
	task := o.taskFor(t, readers.rep1, "Send the pricing sheet", dayNow.Add(2*time.Hour))

	before, ok := rowOf(o.page(t, readers.rep1, "mine", dayNow), task)
	if !ok || before.DueGroup == nil || *before.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupToday {
		t.Fatalf("before the day passed the task reads %+v, want due today", before.DueGroup)
	}
	page := o.page(t, readers.rep1, "mine", dayNow.AddDate(0, 0, 2))
	after, ok := rowOf(page, task)
	if !ok || after.DueGroup == nil || *after.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupOverdue {
		t.Errorf("two days later the task reads %+v, want overdue", after.DueGroup)
	}
	if got := rowIDs(page); !slices.Equal(got, []string{task}) {
		t.Errorf("two days later the day holds %v, want the same single task", got)
	}
}

// A buyer's request captured from the rep's mailbox becomes the rep's task.
// Their colleague's day neither holds it nor carries its words. Capturing the
// same message again changes nothing.
func TestACapturedRequestIsTheOwnersAndSurvivesAReplay(t *testing.T) {
	o := setupOwed(t)
	readers := o.day(t)
	o.contact(t, "Buyer", "buyer@customer.example")
	subject := "Proposal for the integration project"
	mail := owedMail{from: "buyer@customer.example", to: o.seat, subject: subject,
		body: "Please send the proposal for our integration project.", at: o.now.Add(-time.Hour),
		messageID: ids.NewV7().String() + "@customer.example"}
	message := o.capture(t, mail)
	if _, err := o.e.Activities.SetCaptureLabel(o.judge(), message, "commitment"); err != nil {
		t.Fatal(err)
	}
	runOwedWorker(t, o.e, &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: .99})
	request := o.e.WsScalar(t, `SELECT id::text FROM activity WHERE source_system = $1 AND source_activity_id = $2 AND archived_at IS NULL`,
		activities.EmailRequestTaskSource, message)

	owner := o.page(t, readers.rep1, "mine", o.now)
	if got := rowIDs(owner); !slices.Equal(got, []string{request}) {
		t.Errorf("the owner's day holds %v, want the request task %s", got, request)
	}
	if row, ok := rowOf(owner, request); ok {
		if row.Owner == nil || row.Owner.Id == nil || ids.UUID(*row.Owner.Id) != o.e.Rep1 {
			t.Errorf("the request names owner %+v, want rep 1", row.Owner)
		}
		if row.SourceRecord == nil || ids.UUID(row.SourceRecord.Id) != message {
			t.Errorf("the request cites %+v, want the captured mail %s", row.SourceRecord, message)
		}
		if len(row.Actions) == 0 {
			t.Error("the request offers the owner no action")
		}
	}
	colleague := o.page(t, readers.rep2, "mine", o.now)
	if _, ok := rowOf(colleague, request); ok {
		t.Error("the colleague's day holds the owner's request")
	}
	if words, err := json.Marshal(colleague); err != nil || strings.Contains(string(words), subject) {
		t.Errorf("the colleague's day carries the request's subject (err %v)", err)
	}
	// The manager's team day is not asserted. It shows the mail itself as
	// waiting, because the request task is readable by its assignee alone.

	o.capture(t, mail)
	runOwedWorker(t, o.e, &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: .99})
	if got := rowIDs(o.page(t, readers.rep1, "mine", o.now)); !slices.Equal(got, []string{request}) {
		t.Errorf("after a replay the owner's day holds %v, want the same request task", got)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = $1 AND source_activity_id = $2`,
		activities.EmailRequestTaskSource, message); n != 1 {
		t.Errorf("after a replay %d request tasks exist for the mail, want 1", n)
	}
}
