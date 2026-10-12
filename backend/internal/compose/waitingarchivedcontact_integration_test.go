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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingSurfaces is what each reader of the waiting statement says about one
// message. The readers are the Worklist lane, the queue's count, the
// timeline's waiting filter and the owed-verdict backlog.
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

// The message waits, and every reader drops it once the contact is archived.
// Every reader has it again once the contact is restored.
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
	asked := o.ccMail(t, "pat@customer.example", "robin@customer.example", "Rollout dates")
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

// ccMail captures a mail from one customer to the seat with a second on copy.
func (o *owedEnv) ccMail(t *testing.T, from, cc, subject string) ids.UUID {
	t.Helper()
	return o.capture(t, owedMail{
		from: from, to: o.seat, cc: cc, subject: subject, at: o.now.Add(-3 * time.Hour),
		messageID: "in-" + ids.NewV7().String() + "@customer.example",
	})
}

// A contact the reader may not see never decides whether the row shows. The
// copied contact is live but private to a colleague. To this reader the
// message is filed only under its archived sender, so it leaves the lane.
func TestAContactTheReaderCannotSeeDoesNotKeepTheMessage(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	robin := o.contact(t, "Robin Buyer", "robin@customer.example")
	asked := o.ccMail(t, "pat@customer.example", "robin@customer.example", "Rollout dates")
	o.e.MakeCapturePrivate(t, "contact", robin, o.e.Rep3)
	if !o.waiting(t, asked) {
		t.Fatal("before the archive the message is not waiting; the case below would pass for the wrong reason")
	}

	o.archiveContact(t, pat)

	if o.waiting(t, asked) {
		t.Fatal("the message stayed on the lane because of a live contact this reader cannot see")
	}
}

// The reply is owed by the named contact's owner. An unowned live contact is
// named, so the archived sender's owner does not owe it.
func TestTheArchivedContactsOwnerDoesNotOweTheReply(t *testing.T) {
	o := setupOwed(t)
	rep2 := ids.From[ids.UserKind](o.e.Rep2)
	created, err := o.e.Contacts.CreateContact(o.e.Admin(), contacts.CreateContactInput{
		FullName: "Pat Buyer", Source: "manual", OwnerID: &rep2,
		Emails: []contacts.ContactEmailInput{{Email: "pat@customer.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the owned sender: %v", err)
	}
	pat := ids.UUID(created.Id)
	created, err = o.e.Contacts.CreateContact(machineCtx(o.e), contacts.CreateContactInput{
		FullName: "Robin Buyer", Source: "import",
		Emails: []contacts.ContactEmailInput{{Email: "robin@customer.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the unowned copied contact: %v", err)
	}
	robin := ids.UUID(created.Id)
	if created.OwnerId != nil {
		t.Fatalf("the copied contact is owned by %v; the case needs it unowned", *created.OwnerId)
	}
	asked := o.ccMail(t, "pat@customer.example", "robin@customer.example", "Rollout dates")
	if row, ok := o.waitingRow(t, asked); !ok || row.OwnerID != o.e.Rep2 {
		t.Fatalf("before the archive the row is %+v (present=%v); want the sender's owner %v to owe it", row, ok, o.e.Rep2)
	}

	o.archiveContact(t, pat)

	row, ok := o.waitingRow(t, asked)
	if !ok || row.ContactID != robin {
		t.Fatalf("after the archive the row is %+v (present=%v); want it naming the live contact %v", row, ok, robin)
	}
	if row.OwnerID == o.e.Rep2 {
		t.Fatalf("the reply is owed by %v, the archived sender's owner, rather than by whoever owns the named contact", row.OwnerID)
	}
}
