// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type storedAnswerFixture struct {
	e     *resolveEnv
	store *Store
	rep   context.Context
}

func (f storedAnswerFixture) seat(user ids.UUID) context.Context {
	return principal.WithActor(f.e.systemCtx(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull, Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true}, "contact": {Read: true, Update: true}}}})
}

// task writes the rep's open task on a fresh contact reached at address.
func (f storedAnswerFixture) task(t *testing.T, address string) (ids.UUID, time.Time) {
	t.Helper()
	contact := f.e.seedContact(t)
	f.e.exec(t, `INSERT INTO contact_email (contact_id,email,email_type,source,captured_by) VALUES ($1,$2,'work','manual','human:test')`, contact, address)
	task, _, err := f.store.LogActivity(f.rep, LogActivityInput{Kind: "task", Subject: new("Send the file"), Source: "manual", AssigneeID: new(ids.From[ids.UserKind](f.e.rep)), Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(task.Id), task.CreatedAt
}

// mail stores an outbound mail the way capture does, without running the rule.
func (f storedAnswerFixture) mail(as context.Context, t *testing.T, to string, at time.Time) ids.UUID {
	t.Helper()
	return f.addressed(as, t, []string{to}, nil, at)
}

func (f storedAnswerFixture) addressed(as context.Context, t *testing.T, to, cc []string, at time.Time) ids.UUID {
	t.Helper()
	direction := "outbound"
	mail, _, err := f.store.LogActivity(as, LogActivityInput{Kind: "email", Subject: new("The file"), Direction: &direction, OccurredAt: &at, EmailFrom: "rep@resolve.test", EmailTo: to, EmailCc: cc, CounterpartyOutboundAttested: true, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(mail.Id)
}

func (f storedAnswerFixture) run(t *testing.T, limit int) int {
	t.Helper()
	completed := 0
	after := ids.UUID{}
	for {
		var scanned []ids.UUID
		err := f.store.tx(f.e.systemCtx(), func(tx pgx.Tx) error {
			var n int
			var err error
			scanned, n, err = f.store.CompleteAnsweredTasksTx(f.e.systemCtx(), tx, after, limit)
			completed += n
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(scanned) > limit {
			t.Fatalf("a page of %d inspected %d tasks", limit, len(scanned))
		}
		if len(scanned) == 0 {
			return completed
		}
		after = scanned[len(scanned)-1]
	}
}

func TestStoredMailClosesTheTasksItAnsweredOnce(t *testing.T) {
	e := setupResolve(t)
	colleague := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`, colleague, "colleague-"+colleague.String()+"@resolve.test")
	f := storedAnswerFixture{e: e, store: NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))}
	f.rep = f.seat(e.rep)

	answered, created := f.task(t, "answered@stored.test")
	f.mail(f.rep, t, "answered@stored.test", created.Add(time.Hour))
	newest := f.mail(f.rep, t, "answered@stored.test", created.Add(2*time.Hour))
	f.mail(f.seat(colleague), t, "answered@stored.test", created.Add(3*time.Hour))

	earlier, created := f.task(t, "earlier@stored.test")
	f.mail(f.rep, t, "earlier@stored.test", created.Add(-48*time.Hour))
	elsewhere, created := f.task(t, "elsewhere@stored.test")
	f.mail(f.rep, t, "someone-else@stored.test", created.Add(time.Hour))
	byColleague, created := f.task(t, "colleague@stored.test")
	f.mail(f.seat(colleague), t, "colleague@stored.test", created.Add(time.Hour))
	bulk, created := f.task(t, "bulk@stored.test")
	e.exec(t, `UPDATE activity SET bulk_mail_attested=true WHERE id=$1`, f.mail(f.rep, t, "bulk@stored.test", created.Add(time.Hour)))
	blind, created := f.task(t, "blind@stored.test")
	hidden := f.mail(f.rep, t, "blind@stored.test", created.Add(time.Hour))
	e.exec(t, `DELETE FROM activity_participant WHERE activity_id=$1 AND role IN ('to','cc')`, hidden)
	e.exec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'blind@stored.test', 'bcc')`, hidden)

	undone, created := f.task(t, "undone@stored.test")
	f.mail(f.rep, t, "undone@stored.test", created.Add(time.Hour))
	if n, err := f.store.completeContactTasksAnsweredBy(e.systemCtx(), ids.From[ids.ActivityKind](f.mail(f.rep, t, "undone@stored.test", created.Add(2*time.Hour)))); err != nil || n != 1 {
		t.Fatalf("live rule completed %d: %v", n, err)
	}
	if _, err := f.store.UpdateActivity(f.rep, ids.From[ids.ActivityKind](undone), UpdateActivityInput{IsDone: new(false)}); err != nil {
		t.Fatal(err)
	}
	if n, err := f.store.completeContactTasksAnsweredBy(e.systemCtx(), ids.From[ids.ActivityKind](f.mail(f.rep, t, "undone@stored.test", created.Add(3*time.Hour)))); err != nil || n != 0 {
		t.Fatalf("a mail after the reopen closed the task again: %d, %v", n, err)
	}
	copied, created := f.task(t, "copied@stored.test")
	f.addressed(f.rep, t, []string{"someone-else@stored.test"}, []string{"copied@stored.test"}, created.Add(time.Hour))
	among, created := f.task(t, "among@stored.test")
	f.addressed(f.rep, t, []string{"someone-else@stored.test", "among@stored.test"}, nil, created.Add(time.Hour))

	if n := f.run(t, 2); n != 2 {
		t.Fatalf("the batch completed %d tasks, want 2", n)
	}
	for name, closed := range map[string]ids.UUID{"stored mail": answered, "one of several To recipients": among} {
		if !e.isDone(t, closed) {
			t.Errorf("a task answered by %s stayed open", name)
		}
	}
	for name, open := range map[string]ids.UUID{"earlier mail": earlier, "another contact": elsewhere, "a colleague's mail": byColleague, "bulk mail": bulk, "Bcc-only mail": blind, "Cc-only mail": copied, "an undone completion": undone} {
		if e.isDone(t, open) {
			t.Errorf("%s closed its task", name)
		}
	}
	receipts, err := f.store.RecentTaskCompletions(f.rep, created.Add(-time.Hour), 10)
	cited := map[ids.UUID]ids.UUID{}
	for _, receipt := range receipts {
		cited[receipt.ID] = receipt.EmailID
	}
	if err != nil || len(receipts) != 2 || cited[answered] != newest {
		t.Fatalf("receipts=%+v, %v; want one per closed task, citing the newest mail", receipts, err)
	}
	if n := f.run(t, 2); n != 0 {
		t.Fatalf("a second run completed %d tasks", n)
	}
}
