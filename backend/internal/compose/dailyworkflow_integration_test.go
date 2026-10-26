// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A day as two reps and their manager read it. The records are written by the
// native writers: tasks through the activity store, mail through the capture
// sink and the reply classifier. The clock is the feed's own, and advancing it
// is how a day passes.

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dayGrants are what the seeded rep and manager roles hold over the records a
// day is made of.
var dayGrants = map[string]principal.ObjectGrant{
	"activity": {Create: true, Read: true, Update: true}, "contact": {Create: true, Read: true, Update: true},
	"company": {Read: true}, "deal": {Create: true, Read: true, Update: true}, "relationship": {Read: true},
	"installation_settings": {Read: true},
}

type dayReaders struct {
	rep1, rep2, manager func() principal.Principal
}

func (o *owedEnv) day(t *testing.T) (dayReaders, *time.Time, *attentionFeed) {
	t.Helper()
	now := o.now
	// The manager leads the reps' team, so their team day is the reps' work.
	o.e.WsExec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, o.e.Team1, o.e.Rep3)
	rep := func(user ids.UUID) func() principal.Principal {
		return func() principal.Principal {
			actor, _ := principal.Actor(o.e.As(user, []ids.UUID{o.e.Team1},
				principal.Permissions{RoleKeys: []string{"rep"}, Objects: dayGrants, RowScope: principal.RowScopeOwn}))
			return actor
		}
	}
	manager := func() principal.Principal {
		actor, _ := principal.Actor(o.e.As(o.e.Rep3, []ids.UUID{o.e.Team1},
			principal.Permissions{RoleKeys: []string{"manager"}, Objects: dayGrants, RowScope: principal.RowScopeTeam}))
		return actor
	}
	feed := &attentionFeed{o: o, now: &now}
	return dayReaders{rep1: rep(o.e.Rep1), rep2: rep(o.e.Rep2), manager: manager}, &now, feed
}

type attentionFeed struct {
	o   *owedEnv
	now *time.Time
}

// page reads the reader's Worklist at the feed's clock.
func (f *attentionFeed) page(t *testing.T, reader principal.Principal, scope string) crmcontracts.Worklist {
	t.Helper()
	feed := newAttentionService(f.o.e.Pool, approvals.NewService(f.o.e.DB()), func() time.Time { return *f.now })
	ctx := principal.WithActor(f.o.e.As(reader.UserID, nil, reader.Permissions), reader)
	page, err := feed.Worklist(ctx, scope, "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading %s's %s Worklist: %v", reader.UserID, scope, err)
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

func (o *owedEnv) taskFor(t *testing.T, assignee ids.UUID, subject string, due time.Time) string {
	t.Helper()
	user := ids.From[ids.UserKind](assignee)
	ctx := o.e.As(assignee, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	task, _, err := o.e.Activities.LogActivity(ctx, activities.LogActivityInput{
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
	readers, now, feed := o.day(t)
	first := o.taskFor(t, o.e.Rep1, "Send the pricing sheet", now.Add(2*time.Hour))
	second := o.taskFor(t, o.e.Rep2, "Book the onsite visit", now.Add(3*time.Hour))

	if got := rowIDs(feed.page(t, readers.rep1(), "mine")); !slices.Equal(got, []string{first}) {
		t.Errorf("rep 1's day holds %v, want only their task %s", got, first)
	}
	if got := rowIDs(feed.page(t, readers.rep2(), "mine")); !slices.Equal(got, []string{second}) {
		t.Errorf("rep 2's day holds %v, want only their task %s", got, second)
	}
	team := rowIDs(feed.page(t, readers.manager(), "team"))
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
	readers, now, feed := o.day(t)
	task := o.taskFor(t, o.e.Rep1, "Send the pricing sheet", now.Add(2*time.Hour))

	before, ok := rowOf(feed.page(t, readers.rep1(), "mine"), task)
	if !ok || before.DueGroup == nil || *before.DueGroup == crmcontracts.WorklistItemDueGroupWorklistDueGroupOverdue {
		t.Fatalf("before the day passed the task reads %+v, want it due and not overdue", before.DueGroup)
	}
	*now = now.AddDate(0, 0, 2)
	page := feed.page(t, readers.rep1(), "mine")
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
	readers, _, feed := o.day(t)
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

	if got := rowIDs(feed.page(t, readers.rep1(), "mine")); !slices.Equal(got, []string{request}) {
		t.Errorf("the owner's day holds %v, want the request task %s", got, request)
	}
	colleague := feed.page(t, readers.rep2(), "mine")
	if _, ok := rowOf(colleague, request); ok {
		t.Error("the colleague's day holds the owner's request")
	}
	if words, err := json.Marshal(colleague); err != nil || strings.Contains(string(words), subject) {
		t.Errorf("the colleague's day carries the request's subject (err %v)", err)
	}
	// The manager's team day is left unasserted here: it shows the mail as
	// customer_waiting while the rep holds the task, which is #9178.

	o.capture(t, mail)
	runOwedWorker(t, o.e, &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: .99})
	if got := rowIDs(feed.page(t, readers.rep1(), "mine")); !slices.Equal(got, []string{request}) {
		t.Errorf("after a replay the owner's day holds %v, want the same request task", got)
	}
}
