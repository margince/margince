// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An archived contact's open message is not work. Every reader built on the
// waiting statement drops it, and restoring the contact brings it back. The
// mail comes in through the capture sink and the contact is archived and
// restored through the contacts module's own writers.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingSurfaces is what each reader of the waiting statement says about one
// message: the Worklist lane, the queue's count, the timeline's waiting
// filter and the owed-verdict backlog.
type waitingSurfaces struct {
	lane, timeline, backlog bool
	count                   int
}

func (o *owedEnv) surfaces(t *testing.T, message ids.UUID) waitingSurfaces {
	t.Helper()
	store := activities.NewStore(o.e.DB())
	now := time.Now()
	out := waitingSurfaces{lane: o.waiting(t, message)}
	hidden, err := store.HiddenWaiting(o.reader(), now)
	if err != nil {
		t.Fatalf("counting the queue: %v", err)
	}
	out.count = hidden.Shown
	listed, _, err := store.ListActivities(o.reader(), activities.ListActivitiesInput{WaitingReplyAsOf: &now})
	if err != nil {
		t.Fatalf("reading the timeline's waiting filter: %v", err)
	}
	for _, row := range listed {
		out.timeline = out.timeline || ids.UUID(row.Id) == message
	}
	backlog, _, err := store.OwedBacklog(o.judge(), "prompts-test", now, 100, 400, 400)
	if err != nil {
		t.Fatalf("reading the owed-verdict backlog: %v", err)
	}
	for _, row := range backlog {
		out.backlog = out.backlog || row.ID == message
	}
	return out
}

func (o *owedEnv) archiveContact(t *testing.T, contact ids.UUID) {
	t.Helper()
	if _, err := o.e.Contacts.ArchiveContact(o.e.Admin(), ids.From[ids.ContactKind](contact), nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
}

func (o *owedEnv) restoreContact(t *testing.T, contact ids.UUID) {
	t.Helper()
	admin := o.e.Admin()
	if err := database.WithWorkspaceTx(admin, o.e.Pool, func(tx pgx.Tx) error {
		_, err := o.e.Contacts.RestoreContactTx(admin, tx, ids.From[ids.ContactKind](contact), nil, unarchiveWith(nil))
		return err
	}); err != nil {
		t.Fatalf("restoring the contact: %v", err)
	}
}

func (o *owedEnv) waitingRow(t *testing.T, message ids.UUID) (activities.WaitingReply, bool) {
	t.Helper()
	rows, err := activities.NewStore(o.e.DB()).WaitingReplies(o.reader(), time.Now())
	if err != nil {
		t.Fatalf("reading the waiting lane: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == message {
			return row, true
		}
	}
	return activities.WaitingReply{}, false
}

// The issue's five steps: the message waits, the contact is archived and every
// reader drops it, the contact is restored and every reader has it again.
func TestArchivingTheContactTakesTheirMessageOffEveryWaitingReader(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	asked := o.customerWrites(t, "pat@customer.example", "Pricing for next year", o.now.Add(-3*time.Hour))

	live := o.surfaces(t, asked)
	if !live.lane || !live.timeline || !live.backlog || live.count != 1 {
		t.Fatalf("before the archive the readers say %+v; want the message on every one and a count of 1", live)
	}

	o.archiveContact(t, pat)
	if got := o.surfaces(t, asked); got.lane || got.timeline || got.backlog || got.count != 0 {
		t.Fatalf("after the archive the readers say %+v; want the message on none and a count of 0", got)
	}

	o.restoreContact(t, pat)
	if got := o.surfaces(t, asked); got != live {
		t.Fatalf("after the restore the readers say %+v; want what they said before the archive, %+v", got, live)
	}
}

// One live contact keeps the message, and the row then names the live one, so
// its reply opens a record somebody can open.
func TestAMessageStaysWhileOneContactItIsFiledUnderIsLive(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	robin := o.contact(t, "Robin Buyer", "robin@customer.example")
	asked := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, cc: "robin@customer.example",
		subject: "Rollout dates", at: o.now.Add(-3 * time.Hour),
		messageID: "in-" + ids.NewV7().String() + "@customer.example",
	})
	if row, ok := o.waitingRow(t, asked); !ok || row.ContactID != pat {
		t.Fatalf("before the archive the row is %+v (present=%v); want it naming the sender %v", row, ok, pat)
	}

	o.archiveContact(t, pat)

	row, ok := o.waitingRow(t, asked)
	if !ok {
		t.Fatal("the message left the lane although a contact it is filed under is still live")
	}
	if row.ContactID != robin {
		t.Fatalf("the row names %v; want the live contact %v rather than the archived sender %v", row.ContactID, robin, pat)
	}
}
