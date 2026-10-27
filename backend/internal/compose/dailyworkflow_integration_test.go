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
func (o *owedEnv) page(reader context.Context, t *testing.T, scope string, now time.Time) crmcontracts.Worklist {
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
func (o *owedEnv) taskFor(rep context.Context, t *testing.T, subject string, due time.Time) string {
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
	first := o.taskFor(readers.rep1, t, "Send the pricing sheet", dayNow.Add(2*time.Hour))
	second := o.taskFor(readers.rep2, t, "Book the onsite visit", dayNow.Add(3*time.Hour))

	if got := rowIDs(o.page(readers.rep1, t, "mine", dayNow)); !slices.Equal(got, []string{first}) {
		t.Errorf("rep 1's day holds %v, want only their task %s", got, first)
	}
	if got := rowIDs(o.page(readers.rep2, t, "mine", dayNow)); !slices.Equal(got, []string{second}) {
		t.Errorf("rep 2's day holds %v, want only their task %s", got, second)
	}
	team := rowIDs(o.page(readers.manager, t, "team", dayNow))
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
	task := o.taskFor(readers.rep1, t, "Send the pricing sheet", dayNow.Add(2*time.Hour))

	before, ok := rowOf(o.page(readers.rep1, t, "mine", dayNow), task)
	if !ok || before.DueGroup == nil || *before.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupToday {
		t.Fatalf("before the day passed the task reads %+v, want due today", before.DueGroup)
	}
	page := o.page(readers.rep1, t, "mine", dayNow.AddDate(0, 0, 2))
	after, ok := rowOf(page, task)
	if !ok || after.DueGroup == nil || *after.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupOverdue {
		t.Errorf("two days later the task reads %+v, want overdue", after.DueGroup)
	}
	if got := rowIDs(page); !slices.Equal(got, []string{task}) {
		t.Errorf("two days later the day holds %v, want the same single task", got)
	}
}

// A buyer's request captured from the rep's mailbox becomes the rep's task.
// Their colleague's day neither holds it nor carries its words. The manager's
// team day holds the same task, not the mail as if nobody were on it.
// Capturing the same message again changes nothing.
func TestACapturedRequestIsTheOwnersAndSurvivesAReplay(t *testing.T) {
	o := setupOwed(t)
	readers := o.day(t)
	o.contact(t, "Buyer", "buyer@customer.example")
	subject := "Proposal for the integration project"
	mail := owedMail{
		from: "buyer@customer.example", to: o.seat, subject: subject,
		body: "Please send the proposal for our integration project.", at: o.now.Add(-time.Hour),
		messageID: ids.NewV7().String() + "@customer.example",
	}
	message := o.capture(t, mail)
	if _, err := o.e.Activities.SetCaptureLabel(o.judge(), message, "commitment"); err != nil {
		t.Fatal(err)
	}
	runOwedWorker(t, o.e, &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: .99})
	request := o.e.WsScalar(t, `SELECT id::text FROM activity WHERE source_system = $1 AND source_activity_id = $2 AND archived_at IS NULL`,
		activities.EmailRequestTaskSource, message)

	owner := o.page(readers.rep1, t, "mine", o.now)
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
	colleague := o.page(readers.rep2, t, "mine", o.now)
	if _, ok := rowOf(colleague, request); ok {
		t.Error("the colleague's day holds the owner's request")
	}
	if words, err := json.Marshal(colleague); err != nil || strings.Contains(string(words), subject) {
		t.Errorf("the colleague's day carries the request's subject (err %v)", err)
	}
	team := o.page(readers.manager, t, "team", o.now)
	if _, ok := rowOf(team, request); !ok {
		t.Errorf("the manager's team day holds %v, want the rep's request task %s", rowIDs(team), request)
	}
	if _, ok := rowOf(team, message.String()); ok {
		t.Error("the manager's team day shows the mail as waiting while the rep holds its task")
	}

	o.capture(t, mail)
	runOwedWorker(t, o.e, &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: .99})
	if got := rowIDs(o.page(readers.rep1, t, "mine", o.now)); !slices.Equal(got, []string{request}) {
		t.Errorf("after a replay the owner's day holds %v, want the same request task", got)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = $1 AND source_activity_id = $2`,
		activities.EmailRequestTaskSource, message); n != 1 {
		t.Errorf("after a replay %d request tasks exist for the mail, want 1", n)
	}
}

// done completes a task the way a rep does, through the activity writer.
func (o *owedEnv) done(t *testing.T, task string) {
	t.Helper()
	finished := true
	if _, err := o.e.Activities.UpdateActivity(o.e.Admin(),
		ids.From[ids.ActivityKind](ids.MustParse(task)),
		activities.UpdateActivityInput{IsDone: &finished}); err != nil {
		t.Fatalf("completing %s: %v", task, err)
	}
}

// considered totals what the page says it read, across every category. A count
// answered from anywhere but this page's own rows shows up here.
func considered(page crmcontracts.Worklist) int {
	total := 0
	for _, count := range page.Counts {
		total += count.Considered
	}
	return total
}

// A day with nothing owed holds nothing, and the days after a task is done
// stay that way. A count that answered from a stale read would still show the
// finished task while the queue no longer does.
func TestQuietDaysHoldNothingAndInventNothing(t *testing.T) {
	o := setupOwed(t)
	readers := o.day(t)
	task := o.taskFor(readers.rep1, t, "Send the pricing sheet", dayNow.Add(2*time.Hour))
	if _, ok := rowOf(o.page(readers.rep1, t, "mine", dayNow), task); !ok {
		t.Fatalf("the task is not on the day it is due, so the quiet days below prove nothing")
	}
	o.done(t, task)

	for _, after := range []int{0, 1, 2, 7} {
		when := dayNow.AddDate(0, 0, after)
		page := o.page(readers.rep1, t, "mine", when)
		if got := rowIDs(page); len(got) != 0 {
			t.Errorf("day %d holds %v, want nothing once the task is done", after, got)
		}
		if n := considered(page); n != 0 {
			t.Errorf("day %d counts %d item(s) read, want none", after, n)
		}
	}
	// The manager's team day answers the same, from its own read.
	team := o.page(readers.manager, t, "team", dayNow.AddDate(0, 0, 1))
	if got := rowIDs(team); len(got) != 0 {
		t.Errorf("the manager's quiet day holds %v, want nothing", got)
	}
}

// A reply is not the same as a fulfillment. The settlement verdict tells them
// apart, and only one of the three stops the work being owed.
//
// A reply that left something undone keeps it owed. So does a model that would
// not commit.
func TestOnlyAFulfilledRequestStopsBeingOwed(t *testing.T) {
	for _, c := range []struct {
		verdict   string
		stillOwed bool
	}{
		{activities.RequestSettled, false},
		{activities.RequestStillOwed, true},
		{activities.RequestUnsure, true},
	} {
		t.Run(c.verdict, func(t *testing.T) {
			o := setupOwed(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			request := o.askOf(t, "Signed NDA", o.now.Add(-3*time.Hour))
			o.weWrite(t, owedMail{
				to: "pat@customer.example", subject: "Re: Signed NDA", at: o.now.Add(-2 * time.Hour),
			})
			candidate, ok := o.offered(t, request)
			if !ok {
				t.Fatal("the answered request was never offered for judgement")
			}
			if !o.owed(t, request) {
				t.Fatal("the request stopped being owed before anything judged the reply")
			}

			if err := activities.NewStore(o.e.DB()).SettleRequest(o.judge(), activities.RequestSettlementInput{
				Request: candidate, Verdict: c.verdict, Confidence: 0.95, DecidedBy: "test-model",
			}); err != nil {
				t.Fatalf("settling as %s: %v", c.verdict, err)
			}
			if got := o.owed(t, request); got != c.stillOwed {
				t.Errorf("after %s the request reads owed=%v, want %v", c.verdict, got, c.stillOwed)
			}
		})
	}
}

// dueGroup names a row's due group for a failure message. The field is a
// pointer, so a bare print gives an address where a reader wants a word.
func dueGroup(row crmcontracts.WorklistItem) string {
	if row.DueGroup == nil {
		return "none"
	}
	return string(*row.DueGroup)
}

// The week closing moves the same work, it does not replace it.
//
// A Friday task read on the Wednesday is that week's. Read on the Monday
// after, it is overdue, and still the same row, still the rep's, and still on
// their manager's team day.
func TestTheWeekClosingKeepsTheWorkAndItsReaders(t *testing.T) {
	o := setupOwed(t)
	readers := o.day(t)
	// dayNow is a Wednesday, so Friday afternoon is this week and not tomorrow.
	friday := dayNow.AddDate(0, 0, 2).Add(5 * time.Hour)
	task := o.taskFor(readers.rep1, t, "Send the pricing sheet", friday)

	open, ok := rowOf(o.page(readers.rep1, t, "mine", dayNow), task)
	if !ok || open.DueGroup == nil ||
		*open.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupThisWeek {
		t.Fatalf("while the week is open the task reads %s, want this week", dueGroup(open))
	}
	if open.Owner == nil {
		t.Fatal("the task names no owner while the week is open")
	}

	// The Monday after: the week has closed on the clock alone.
	monday := dayNow.AddDate(0, 0, 5)
	page := o.page(readers.rep1, t, "mine", monday)
	closed, ok := rowOf(page, task)
	if !ok || closed.DueGroup == nil ||
		*closed.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupOverdue {
		t.Fatalf("once the week closed the task reads %s, want overdue", dueGroup(closed))
	}
	if got := rowIDs(page); !slices.Equal(got, []string{task}) {
		t.Errorf("the new week holds %v, want the same single task", got)
	}
	// The owner is compared by who it names. Its id and label are pointers, so
	// the structs differ by address on every read even when nothing moved.
	if closed.Owner == nil || closed.Owner.Id == nil || open.Owner.Id == nil ||
		*closed.Owner.Id != *open.Owner.Id || closed.Owner.Kind != open.Owner.Kind {
		t.Errorf("the week closed and the task answers to %+v, want %+v", closed.Owner, open.Owner)
	}

	team := o.page(readers.manager, t, "team", monday)
	if _, ok := rowOf(team, task); !ok {
		t.Errorf("the manager's team day holds %v, want the carried task %s", rowIDs(team), task)
	}
}
