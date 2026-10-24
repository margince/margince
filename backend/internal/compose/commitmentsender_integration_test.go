// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Colleague P writes to a customer and copies colleague M. Only M's copy is
// captured, and the customer's record is private to M. P made a promise in the
// mail. P gets the work and may open that one message, because P sent it.
// Nobody else gains anything.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	senderPromise = "I will send the signed order form by Friday."
	// senderThreadWord is in the customer's reply alone, which the sender may
	// not open. It must reach nothing the sender reads.
	senderThreadWord = "Zanzibar"
	senderThread     = "thread-sender@buyer.example"
	senderOwnDomain  = "authz.test"
)

// senderReadPerms is a colleague who reads mail and contacts within their team.
var senderReadPerms = principal.Permissions{
	Objects: map[string]principal.ObjectGrant{
		"activity": {Create: true, Read: true, Update: true},
		"contact":  {Read: true, Update: true},
		"company":  {Read: true},
		"approval": {Read: true, Update: true},
	},
	RowScope: principal.RowScopeTeam,
}

type senderMail struct {
	e                 *integration.Env
	owner, sender     ids.UUID
	sent, reply, ines ids.UUID
}

// seedSenderMail captures mail through the real sink into M's (Rep1) mailbox.
// The first is P's (Rep2) mail to the customer with M on Cc. The second is the
// customer's reply to M. The customer is private to M, so the thread is too.
func seedSenderMail(t *testing.T, trustDomain bool) senderMail {
	t.Helper()
	e := integration.Setup(t)
	// Two colleagues of one name make the promiser ambiguous.
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya' WHERE id = $1`, e.Rep2)
	if trustDomain {
		e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ($1, 'admin', true)`, senderOwnDomain)
	}
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := e.SeedContact(t, "Ines Huber", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, ines, company)
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](ines), contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{{Email: "ines@buyer.example", EmailType: "work", IsPrimary: true}}}); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, ines, e.Rep1)
	m := senderMail{e: e, owner: e.Rep1, sender: e.Rep2, ines: ines}
	m.sent = captureSenderCopy(t, e, e.Rep1, e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, e.Rep2),
		"<"+senderThread+">", "", "Hi Ines, "+senderPromise, clocktest.Now(t).Add(-48*time.Hour))
	m.reply = captureSenderCopy(t, e, e.Rep1, "ines@buyer.example", "<reply-"+senderThread+">", "<"+senderThread+">",
		"Thanks. Our "+senderThreadWord+" office signs on Thursday.", clocktest.Now(t).Add(-47*time.Hour))
	return m
}

// captureSenderCopy is one message as Gmail delivers it to owner's mailbox,
// with Gmail's DMARC pass for the From domain.
func captureSenderCopy(t *testing.T, e *integration.Env, owner ids.UUID, from, messageID, inReplyTo, body string, sent time.Time) ids.UUID {
	t.Helper()
	buyer := "ines@buyer.example"
	domain := from[strings.LastIndex(from, "@")+1:]
	headers := []string{
		"Authentication-Results: mx.google.com; dmarc=pass (p=NONE sp=NONE dis=NONE) header.from=" + domain,
		"From: " + from, "To: " + buyer, "Cc: " + seatAddress(t, e, owner), "Subject: Order form",
		"Date: " + sent.Format(time.RFC1123Z), "Message-ID: " + messageID,
	}
	if inReplyTo != "" {
		headers = append(headers, "In-Reply-To: "+inReplyTo, "References: "+inReplyTo)
	}
	if from == buyer {
		headers[2] = "To: " + seatAddress(t, e, owner)
		headers[3] = "Cc: " + e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, e.Rep2)
	}
	raw := strings.Join(append(headers, "Content-Type: text/plain; charset=utf-8", "", body), "\r\n")
	captureThreadMail(t, e, owner, []byte(raw))
	key := strings.Trim(messageID, "<>")
	message, err := ids.Parse(e.WsScalar(t, `SELECT activity_id::text FROM activity_identity WHERE identity_kind='mail' AND identity_key=$1`, key))
	if err != nil {
		t.Fatalf("the message %s was not captured: %v", key, err)
	}
	return message
}

// offerSenderThread lets the reader read the thread, as its mailbox owner decides.
func (m senderMail) offerSenderThread(t *testing.T) {
	t.Helper()
	reader := principal.WithActor(m.e.As(m.owner, []ids.UUID{m.e.Team1}, integration.AdminPerms), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + m.owner.String(), UserID: m.owner, SeatType: principal.SeatFull,
		Permissions: integration.AdminPerms,
	})
	key := m.e.WsScalar(t, `SELECT thread_key FROM activity WHERE id = $1`, m.sent)
	if _, err := NewThreadAudienceSetter(m.e.Pool).Decide(reader, key, true); err != nil {
		t.Fatal(err)
	}
}

// seedSenderCheck registers the check as an upgrade does, without running it.
func (m senderMail) seedSenderCheck(t *testing.T) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(m.e.Pool, []datatask.Task{colleagueSenderDataTask()}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(principal.SystemActing(m.e.Admin(), "system:datatask-test")); err != nil {
		t.Fatal(err)
	}
	if n := m.e.WsCount(t, `SELECT count(*) FROM data_task_run WHERE step = $1 AND status <> 'done'`, auth.SenderStampsCheckedStep); n != 1 {
		t.Fatalf("the sender check is not waiting to run (%d rows)", n)
	}
}

func (m senderMail) runSenderCheck(t *testing.T) {
	t.Helper()
	runMachineAddressTask(principal.SystemActing(m.e.Admin(), "system:datatask-test"), t, m.e, colleagueSenderDataTask())
	if n := m.e.WsCount(t, `SELECT count(*) FROM data_task_run WHERE step = $1 AND status = 'done'`, auth.SenderStampsCheckedStep); n != 1 {
		t.Fatalf("the sender check did not finish (%d done rows)", n)
	}
}

func (m senderMail) seat(user ids.UUID) context.Context {
	team := m.e.Team1
	if user == m.e.Rep3 {
		team = m.e.Team2
	}
	ctx := m.e.As(user, []ids.UUID{team}, senderReadPerms)
	actor, _ := principal.Actor(ctx)
	actor.SeatType = principal.SeatFull
	return principal.WithActor(ctx, actor)
}

// opens reports whether user may read the activity's content, through the
// store's own read path.
func (m senderMail) opens(t *testing.T, user, activity ids.UUID) bool {
	t.Helper()
	err := database.WithWorkspaceTx(m.seat(user), m.e.Pool, func(tx pgx.Tx) error {
		return auth.EnsureActivityContentVisibleLive(m.seat(user), tx, activity)
	})
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading %s as %s: %v", activity, user, err)
	}
	return err == nil
}

func senderPromiseReply(t *testing.T, message, promiser ids.UUID, confidence float64) string {
	t.Helper()
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": message.String(),
		// The model sums up the thread, the customer's reply included.
		"summary": "Send the order form before the " + senderThreadWord + " signing", "quote": senderPromise,
		"due_date": "", "confidence": confidence, "promiser_ref": "seat:" + promiser.String(),
		"commitment": openCommitmentMeaning("", ""),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(reply)
}

// The sender's sure promise is their task, read by them and the mailbox owner.
// The sender opens their own message and the task, and nothing else on the
// thread. A third colleague finds neither.
func TestTheSenderOfACopiedMailHoldsTheirPromiseAndOpensOnlyThatMail(t *testing.T) {
	m := seedSenderMail(t, true)
	e := m.e
	if got := e.WsScalar(t, `SELECT coalesce(sent_by_colleague::text, '') FROM activity WHERE id = $1`, m.sent); got != m.sender.String() {
		t.Fatalf("the copy names %q as its sender, want the colleague who sent it", got)
	}
	m.seedSenderCheck(t)
	if m.opens(t, m.sender, m.sent) {
		t.Error("the sender opened their copy before the older sender stamps were re-checked")
	}
	m.runSenderCheck(t)
	m.offerSenderThread(t)
	if err := runSignalPass(t, e, senderPromiseReply(t, m.sent, m.sender, taskWorthyConfidence)); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	task, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM activity WHERE kind = 'task' AND assignee_id = $1`, m.sender))
	if err != nil {
		t.Fatalf("the sender has no task for their promise: %v", err)
	}
	if got := e.WsScalar(t, `SELECT string_agg(subject_id::text, ',' ORDER BY subject_id) FROM activity_audience_member WHERE activity_id = $1`, task); got != joinSorted(m.owner, m.sender) {
		t.Errorf("the task is read by %q, want the sender and the mailbox owner alone", got)
	}
	if subject := e.WsScalar(t, `SELECT subject FROM activity WHERE id = $1`, task); strings.Contains(subject, senderThreadWord) || !strings.Contains(subject, "signed order form") {
		t.Errorf("the task reads %q, want the sender's own sentence and nothing from the reply they cannot open", subject)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d request(s) staged beside the sender's own task", n)
	}

	for _, c := range []struct {
		name     string
		user, id ids.UUID
		want     bool
	}{
		{"the sender opens their message", m.sender, m.sent, true},
		{"the sender opens their task", m.sender, task, true},
		{"the sender cannot open the customer's reply", m.sender, m.reply, false},
		{"the sender cannot open the private contact", m.sender, m.ines, false},
		{"the mailbox owner opens the task", m.owner, task, true},
		{"a third colleague cannot open the message", e.Rep3, m.sent, false},
		{"a third colleague cannot find the task", e.Rep3, task, false},
	} {
		if c.id == m.ines {
			if got := contactVisibleTo(t, m, c.user, c.id); got != c.want {
				t.Errorf("%s: visible = %v", c.name, got)
			}
			continue
		}
		if got := m.opens(t, c.user, c.id); got != c.want {
			t.Errorf("%s: opens = %v, want %v", c.name, got, c.want)
		}
	}
	// The task is filed under the customer's company as well, which every
	// colleague may see, so the row's existence is not secret. Its words are.
	if got := e.WsScalar(t, `SELECT array_to_string(evidence_ids, ',') FROM activity_commitment_admission`); got != m.sent.String() {
		t.Errorf("the recorded decision names messages %q, want the sender's own message alone", got)
	}
	got, err := e.Activities.GetActivity(m.seat(e.Rep3), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a third colleague reading the task: %v", err)
	}
	if err == nil && got.Subject != nil {
		t.Errorf("a third colleague read the sender's task: %q", *got.Subject)
	}
	// A domain that stops being ours closes the copy at once, before any check runs.
	e.WsExec(t, `DELETE FROM workspace_email_domain WHERE domain = $1`, senderOwnDomain)
	if m.opens(t, m.sender, m.sent) {
		t.Error("the sender still opens the copy after their domain stopped being one of ours")
	}
}

// An unsure promise is put to the sender alone. They accept it, and the task is
// filed under the records the reader chose, which the sender could not attach.
func TestTheSenderAcceptsTheirOwnPromiseFromAnotherMembersPrivateMail(t *testing.T) {
	m := seedSenderMail(t, true)
	e := m.e
	m.runSenderCheck(t)
	m.offerSenderThread(t)
	if err := runSignalPass(t, e, senderPromiseReply(t, m.sent, m.sender, 0.75)); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM approval WHERE kind = 'commitment_task'
		 AND proposed_change->>'seat_id' = $1 AND on_behalf_of::text = $1`, m.sender.String()))
	if err != nil {
		t.Fatalf("no request names the sender: %v", err)
	}
	if summary := e.WsScalar(t, `SELECT proposed_change->>'summary' FROM approval WHERE id = $1`, id); strings.Contains(summary, senderThreadWord) {
		t.Errorf("the request reads %q, which carries the reply the sender cannot open", summary)
	}
	queue := approvalsServiceWithEffects(e.Pool)
	shown, err := queue.GetWire(m.seat(m.sender), ids.From[ids.ApprovalKind](id))
	if err != nil {
		t.Fatalf("the sender cannot read their own request: %v", err)
	}
	for _, key := range []string{"links", "claim_id", "claim_ids"} {
		if _, ok := (*shown.ProposedChange)[key]; ok {
			t.Errorf("the request shows the sender %q, which names the mailbox owner's private records", key)
		}
	}
	if could, err := queue.PendingDecidableBy(m.seat(m.owner), ids.From[ids.ApprovalKind](id)); err != nil || could {
		t.Errorf("the mailbox owner may decide the sender's promise (could=%v, err=%v)", could, err)
	}
	if _, err := queue.Decide(m.seat(m.sender), ids.From[ids.ApprovalKind](id), true, nil); err != nil {
		t.Fatalf("the sender could not accept their own promise: %v", err)
	}
	task, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM activity WHERE kind = 'task' AND assignee_id = $1`, m.sender))
	if err != nil {
		t.Fatalf("accepting wrote no task for the sender: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, task); n == 0 {
		t.Error("the task was filed under nothing, which any colleague may discover")
	}
	if !m.opens(t, m.sender, task) || m.opens(t, e.Rep3, task) {
		t.Errorf("the task opens for the sender = %v and for a third colleague = %v, want true and false",
			m.opens(t, m.sender, task), m.opens(t, e.Rep3, task))
	}
}

// A seat whose login address is not at one of the workspace's own domains is
// never the sender of mail from that address. DMARC passing does not change it.
// Its mail grants it nothing and gives it no work.
func TestASeatAtAForeignDomainGainsNothingFromMailFromItsAddress(t *testing.T) {
	m := seedSenderMail(t, false)
	e := m.e
	if got := e.WsScalar(t, `SELECT coalesce(sent_by_colleague::text, '') FROM activity WHERE id = $1`, m.sent); got != "" {
		t.Fatalf("mail from a seat at a domain that is not ours names %s as its sender", got)
	}
	m.runSenderCheck(t)
	m.offerSenderThread(t)
	if err := runSignalPass(t, e, senderPromiseReply(t, m.sent, m.sender, taskWorthyConfidence)); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if m.opens(t, m.sender, m.sent) {
		t.Error("a seat at a domain that is not ours opened the mailbox owner's copy")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("%d task(s) written from mail no colleague is proven to have sent", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d request(s) staged from mail no colleague is proven to have sent", n)
	}
}

// A stamp written while the domain counted as ours is turned back once it no
// longer does. The copy is received mail again and the seat is not its sender.
// The seat cannot open it. A second run changes nothing.
func TestTheSenderCheckTurnsBackAStampAtADomainThatIsNotOurs(t *testing.T) {
	m := seedSenderMail(t, true)
	e := m.e
	e.WsExec(t, `DELETE FROM workspace_email_domain WHERE domain = $1`, senderOwnDomain)
	m.runSenderCheck(t)

	if got := e.WsScalar(t, `SELECT direction || ' ' || coalesce(sent_by_colleague::text, '-') || ' ' || counterparty_email FROM activity WHERE id = $1`, m.sent); got != "inbound - b@authz.test" {
		t.Errorf("the copy reads %q, want received mail from the seat's address", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND role = 'from' AND user_id = $2`, m.sent, m.sender); n != 0 {
		t.Error("the seat is still stamped as the copy's sender")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND after->>'direction' = 'inbound'`, m.sent); n != 1 {
		t.Errorf("%d audit row(s) for the turn-back, want one", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text AND envelope->'payload'->'changed_fields'->>'direction' = 'inbound'`, m.sent); n != 1 {
		t.Errorf("%d event(s) for the turn-back, want one", n)
	}
	if m.opens(t, m.sender, m.sent) {
		t.Error("the seat still opens a copy it is no longer proven to have sent")
	}

	step := colleagueSenderDataTask().Steps[0]
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("a second run changed %d row(s)", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func contactVisibleTo(t *testing.T, m senderMail, user, contact ids.UUID) bool {
	t.Helper()
	err := database.WithWorkspaceTx(m.seat(user), m.e.Pool, func(tx pgx.Tx) error {
		return auth.EnsureVisible(m.seat(user), tx, "contact", contact)
	})
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading the contact as %s: %v", user, err)
	}
	return err == nil
}

func joinSorted(a, b ids.UUID) string {
	if b.String() < a.String() {
		a, b = b, a
	}
	return a.String() + "," + b.String()
}

// Every record the task is filed under is private to the mailbox owner. The
// sender still finds their own task, and a third colleague finds no row.
func TestTheSenderFindsTheirTaskWhenEveryRecordItIsFiledUnderIsPrivate(t *testing.T) {
	m := seedSenderMail(t, true)
	e := m.e
	e.WsExec(t, `UPDATE company SET visibility = 'owner', owner_id = $1`, m.owner)
	m.runSenderCheck(t)
	m.offerSenderThread(t)
	if err := runSignalPass(t, e, senderPromiseReply(t, m.sent, m.sender, taskWorthyConfidence)); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	task, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM activity WHERE kind = 'task' AND assignee_id = $1`, m.sender))
	if err != nil {
		t.Fatalf("the sender has no task for their promise: %v", err)
	}
	if !m.opens(t, m.sender, task) {
		t.Error("the sender cannot find their own task")
	}
	if _, err := e.Activities.GetActivity(m.seat(e.Rep3), ids.From[ids.ActivityKind](task), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a third colleague reading the task: err = %v, want not found", err)
	}
	// Handed to the mailbox owner, the task is no longer the sender's to find,
	// though the sender may still open the message it came from.
	owner := ids.From[ids.UserKind](m.owner)
	if _, err := e.Activities.UpdateActivity(m.seat(m.owner), ids.From[ids.ActivityKind](task), activities.UpdateActivityInput{AssigneeID: &owner}); err != nil {
		t.Fatalf("the mailbox owner could not take the task over: %v", err)
	}
	if !m.opens(t, m.sender, m.sent) {
		t.Fatal("the sender can no longer open their message")
	}
	if _, err := e.Activities.GetActivity(m.seat(m.sender), ids.From[ids.ActivityKind](task), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the sender found a task that is no longer theirs: err = %v, want not found", err)
	}
	if _, err := e.Activities.GetActivity(m.seat(e.Rep3), ids.From[ids.ActivityKind](m.sent), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a third colleague learns the sender's message exists: err = %v, want not found", err)
	}
}

// Acceptance reads the copy as it stands. Once it is received mail again, the
// sender may not accept the promise, and the request stays pending.
func TestASenderWhoseCopyWasTurnedBackCannotAcceptThePromise(t *testing.T) {
	m := seedSenderMail(t, true)
	e := m.e
	m.runSenderCheck(t)
	m.offerSenderThread(t)
	if err := runSignalPass(t, e, senderPromiseReply(t, m.sent, m.sender, 0.75)); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM approval WHERE kind = 'commitment_task' AND on_behalf_of = $1`, m.sender))
	if err != nil {
		t.Fatalf("no request names the sender: %v", err)
	}
	e.WsExec(t, `DELETE FROM workspace_email_domain WHERE domain = $1`, senderOwnDomain)
	e.WsExec(t, `DELETE FROM data_task_run WHERE step = $1`, auth.SenderStampsCheckedStep)
	m.runSenderCheck(t)

	if _, err := approvalsServiceWithEffects(e.Pool).Decide(m.seat(m.sender), ids.From[ids.ApprovalKind](id), true, nil); err == nil {
		t.Error("the sender accepted a promise from a copy no longer proven to be theirs")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("%d task(s) written from a copy no longer proven to be the sender's", n)
	}
	if status := e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, id); status != "pending" {
		t.Errorf("the request is %q, want it still pending", status)
	}
}
