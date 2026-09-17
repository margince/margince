// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The immediate email leg's one guarantee: at most one message per notice,
// ever.
//
// It drives the production worker rather than the claim underneath it, for the
// reason the weekly's suite gives: a test calling ClaimEmailAttempt directly
// would prove the UPDATE is conditional and prove nothing about the lane — the
// lane is where a second message would actually come from, because it is the
// thing River hands a redelivered job to.
//
// Every notice here is written by a real producer — the approval fan-out, or
// the store's own Create under the system principal — so nothing below asserts
// against a row the product would never write.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noticeMailOrigin is the installation's own public origin, as an operator
// would configure it.
const noticeMailOrigin = "https://crm.example.test"

// noticeMailEnv is the approval fan-out's world with the mail leg wired over
// it: the same seats, the same staged proposal, and a relay that counts.
type noticeMailEnv struct {
	approvalNotifyEnv
	relay  *countingMailer
	worker *notificationMailWorker
}

func setupNoticeMail(t *testing.T) *noticeMailEnv {
	t.Helper()
	a := newApprovalNotifyEnv(t)
	relay := &countingMailer{}
	return &noticeMailEnv{
		approvalNotifyEnv: a,
		relay:             relay,
		worker: newNotificationMailWorker(a.e.Pool, NotificationMailConfig{
			Mailer: relay, PublicBaseURL: noticeMailOrigin,
		}, slog.New(slog.NewTextHandler(io.Discard, nil))),
	}
}

// pendingNotice stages one proposal the admin can decide and returns the
// notice the fan-out wrote for them.
//
// No preference is set: approval_pending's installation default IS email
// (notices.DefaultDelivery), so this is the seat every other case below varies
// from.
func (e *noticeMailEnv) pendingNotice(t *testing.T) ids.UUID {
	t.Helper()
	e.grantRole(t, e.e.AdminUser)
	_, env := e.stageCorrection(t)
	e.deliver(t, env)
	rows := e.delivered(t)
	if len(rows) != 1 {
		t.Fatalf("the fan-out wrote %d notice(s); this suite needs the one addressed to the admin: %+v", len(rows), rows)
	}
	return rows[0].id
}

// work hands the worker one job, exactly as River would.
func (e *noticeMailEnv) work(t *testing.T, notice ids.UUID) {
	t.Helper()
	if err := e.worker.Work(context.Background(), &river.Job[SendNotificationEmailArgs]{
		Args: SendNotificationEmailArgs{Workspace: e.e.WS, NoticeID: notice.String()},
	}); err != nil {
		t.Fatalf("the notification mail worker refused the job: %v", err)
	}
}

// emailState is the claim and its cause as the row holds them.
func (e *noticeMailEnv) emailState(t *testing.T, notice ids.UUID) (*time.Time, *string) {
	t.Helper()
	var stamp *time.Time
	var cause *string
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT email_attempted_at, email_error FROM notice WHERE id = $1`,
		notice).Scan(&stamp, &cause); err != nil {
		t.Fatalf("reading the notice's mail state: %v", err)
	}
	return stamp, cause
}

// seatAddress is the admin's own address, read rather than restated: the
// harness mints it, and a literal here would pass while the mail went
// somewhere else.
func (e *noticeMailEnv) seatAddress(t *testing.T) string {
	t.Helper()
	var address string
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT email FROM app_user WHERE id = $1`, e.e.AdminUser).Scan(&address); err != nil {
		t.Fatalf("reading the seat's address: %v", err)
	}
	return address
}

// THE GUARANTEE. River redelivers a job on any attempt it cannot confirm
// finished, so the message is offered to the relay more than once. Exactly one
// of those may reach it: a colleague told twice that the same decision is
// waiting learns to read neither message.
func TestAnApprovalWaitingOnAnEmailSeatIsMailedOnce(t *testing.T) {
	e := setupNoticeMail(t)
	notice := e.pendingNotice(t)

	e.work(t, notice)
	e.work(t, notice)

	if got := e.relay.count(); got != 1 {
		t.Fatalf("the relay was handed the notice %d times; the mail is at-most-once", got)
	}
	if want := e.seatAddress(t); e.relay.sends[0] != want {
		t.Errorf("the mail went to %q, not to the seat the notice is addressed to (%q)", e.relay.sends[0], want)
	}
	subject := e.relay.subjects[0]
	if !strings.Contains(subject, stagedSummary) {
		t.Errorf("the subject %q does not carry what the notice says is waiting", subject)
	}
	body := e.relay.bodies[0]
	if !strings.Contains(body, "\n"+stagedSummary+"\n") {
		t.Errorf("the body does not carry the notice's own line:\n%s", body)
	}
	if want := noticeMailOrigin + "/#/worklist"; !strings.Contains(body, want) {
		t.Errorf("the body does not link to the worklist (%q):\n%s", want, body)
	}

	stamp, cause := e.emailState(t, notice)
	if stamp == nil {
		t.Error("a sent notice left no claim, so the next delivery would mail it again")
	}
	if cause != nil {
		t.Errorf("a mail that went out recorded a failure: %q", *cause)
	}
}

// An installation that has not been told its own origin mails the message
// WITHOUT the closing line, rather than a link built on an empty base.
func TestWithNoPublicOriginTheNotificationMailCarriesNoLink(t *testing.T) {
	e := setupNoticeMail(t)
	e.worker.mail.PublicBaseURL = ""
	notice := e.pendingNotice(t)

	e.work(t, notice)

	if got := e.relay.count(); got != 1 {
		t.Fatalf("the notice was handed to the relay %d times, want once", got)
	}
	if body := e.relay.bodies[0]; strings.Contains(body, "/#/") || strings.Contains(body, "http") {
		t.Errorf("the message carries a link built on an empty origin:\n%s", body)
	}
}

// A worker composed with no relay spends nothing.
//
// The leg is off BY OMISSION rather than by a flag, and that property is worth
// a case of its own: the guard sits above the claim, so an installation that
// arms its relay tomorrow still reaches tomorrow's decisions. What it does not
// do is rescue this one — the job finishes, and the notice waits on the
// Worklist like every other notice — which is why the branch says so at Warn
// and api/jobs.yaml waives it by name.
func TestAWorkerWithNoRelaySpendsNoClaim(t *testing.T) {
	e := setupNoticeMail(t)
	notice := e.pendingNotice(t)
	e.worker.mail = NotificationMailConfig{}

	e.work(t, notice)

	if got := e.relay.count(); got != 0 {
		t.Fatalf("a worker with no relay reached one anyway: %v", e.relay.sends)
	}
	stamp, cause := e.emailState(t, notice)
	if stamp != nil {
		t.Error("a worker with no relay burned the notice's attempt, so arming mail later would never reach it")
	}
	if cause != nil {
		t.Errorf("an unconfigured role recorded a failure on somebody's notice: %q", *cause)
	}
}

// A notice the reader already answered on screen is not then mailed about.
//
// The claim's own read_at predicate is what holds this: the job is enqueued
// when the notice is written and may be worked seconds later, and by then the
// colleague may have opened the card. The attempt stays UNSPENT, because
// nothing was attempted.
func TestANoticeAlreadyReadInAppIsNotMailed(t *testing.T) {
	e := setupNoticeMail(t)
	notice := e.pendingNotice(t)

	seatCtx := e.e.As(e.e.AdminUser, nil, integration.AdminPerms)
	if err := notices.NewStore(e.db).MarkRead(seatCtx, notice); err != nil {
		t.Fatalf("settling the notice on screen: %v", err)
	}

	e.work(t, notice)

	if got := e.relay.count(); got != 0 {
		t.Fatalf("a notice already read on screen was mailed: %v", e.relay.sends)
	}
	if stamp, _ := e.emailState(t, notice); stamp != nil {
		t.Error("a notice nothing was attempted for carries a claim")
	}
}

// A seat deactivated between the notice and the send is a SKIP, not a failure.
//
// Deactivating an account leaves archived_at NULL, so a recipient read on
// archived_at alone would mail a departed colleague. Nothing is recorded as a
// cause either: the product did not fail to send, it correctly declined to.
func TestADeactivatedSeatIsNotMailedTheirNotice(t *testing.T) {
	e := setupNoticeMail(t)
	notice := e.pendingNotice(t)
	e.e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.e.AdminUser)

	e.work(t, notice)

	if got := e.relay.count(); got != 0 {
		t.Fatalf("a deactivated seat was mailed: %v", e.relay.sends)
	}
	if _, cause := e.emailState(t, notice); cause != nil {
		t.Errorf("declining to mail a departed colleague was recorded as a failure: %q", *cause)
	}
}

// A relay that refuses the message spends the claim anyway, and says why.
//
// The trade the design accepts rather than a defect: SMTP reports no receipt,
// so a retry could not tell a refused message from a delivered one and would
// risk telling a colleague twice. What must not happen is the failure
// vanishing — the cause is written onto the row, so a missing mail is
// answerable, and the stamp is what stops the next delivery trying again.
func TestARefusedNotificationMailSpendsTheClaimAndRecordsWhy(t *testing.T) {
	e := setupNoticeMail(t)
	e.relay.fail = errors.New("relay refused the recipient")
	notice := e.pendingNotice(t)

	e.work(t, notice)
	e.work(t, notice)

	if got := e.relay.count(); got != 1 {
		t.Fatalf("a refused mail was retried %d times; there is no receipt to retry against", got)
	}
	stamp, cause := e.emailState(t, notice)
	if stamp == nil {
		t.Error("a refused attempt left no claim, so the next delivery would send again")
	}
	if cause == nil || !strings.Contains(*cause, "refused") {
		t.Errorf("the failure was not recorded on the row: %v", cause)
	}
}

// A seat who switched the class to the screen between the notice and the send
// is a SKIP, and the claim stays spent.
//
// Not a failure: nothing went wrong, and recording a cause would put "the
// preference changed" in the place an operator reads to find out why a message
// they expected never arrived.
func TestAPreferenceChangedBeforeTheSendIsASkipAndNotAFailure(t *testing.T) {
	e := setupNoticeMail(t)
	notice := e.pendingNotice(t)

	seatCtx := e.e.As(e.e.AdminUser, nil, integration.AdminPerms)
	if _, err := notices.NewStore(e.db).SaveNotificationPreference(
		seatCtx, notices.ClassApprovalPending, notices.DeliveryInApp); err != nil {
		t.Fatalf("moving the class to the screen: %v", err)
	}

	e.work(t, notice)

	if got := e.relay.count(); got != 0 {
		t.Fatalf("a seat who moved the class off email was mailed anyway: %v", e.relay.sends)
	}
	if _, cause := e.emailState(t, notice); cause != nil {
		t.Errorf("a preference change was recorded as a failure: %q", *cause)
	}
}

// Nothing a notice carries can write a line of its own in the message.
//
// mailer.Send guards the headers; the body is the sender's to keep honest, and
// a notice's subject and body are written by producers that accept a newline —
// the store bounds their length and nothing strips their structure. A forged
// "From:" line in a message a colleague reads as the product's is the whole
// reason every interpolated value goes through OneLine.
func TestTheNotificationMailCarriesNoLineTheNoticeForged(t *testing.T) {
	e := setupNoticeMail(t)
	seatCtx := e.e.As(e.e.AdminUser, nil, integration.AdminPerms)
	if _, err := notices.NewStore(e.db).SaveNotificationPreference(
		seatCtx, "automation", notices.DeliveryEmail); err != nil {
		t.Fatalf("routing the class to email: %v", err)
	}
	// Through the store's own writer under the system principal, the way an
	// automation's notify leg records one.
	notice, err := notices.NewStore(e.db).Create(e.proposerCtx(ids.Nil), notices.NewNotice{
		Recipient: ids.From[ids.UserKind](e.e.AdminUser),
		Kind:      "automation",
		Subject:   "Quota reached\nFrom: finance@margince.test",
		Body:      "the first line\nSubject: something else entirely",
		DedupeKey: "forged:" + e.e.AdminUser.String(),
	})
	if err != nil {
		t.Fatalf("recording the notice: %v", err)
	}

	e.work(t, notice)

	if got := e.relay.count(); got != 1 {
		t.Fatalf("the notice was handed to the relay %d times, want once", got)
	}
	if subject := e.relay.subjects[0]; strings.ContainsAny(subject, "\r\n") {
		t.Errorf("the subject carries a line break: %q", subject)
	}
	body := e.relay.bodies[0]
	for _, forged := range []string{"\nFrom: finance@margince.test", "\nSubject: something else entirely"} {
		if strings.Contains(body, forged) {
			t.Errorf("the notice wrote a line of its own into the message (%q):\n%s", forged, body)
		}
	}
}

// The enqueue's own gate: only a seat who asked for email gets a job at all.
//
// It is the ordering the whole design rests on — the preference is read where
// the notice is written, so a colleague who reads their queue on screen never
// has a durable job staged about them, and the worker's re-read below is only
// the race window's guard.
func TestOnlyAnEmailSeatsNoticeStagesAMailJob(t *testing.T) {
	e := setupNoticeMail(t)
	e.grantRole(t, e.e.AdminUser)

	_, env := e.stageCorrection(t)
	e.deliver(t, env)

	staged := e.queue.staged()
	if len(staged) != 1 {
		t.Fatalf("one seat wants this by mail; %d job(s) were staged: %+v", len(staged), staged)
	}
	rows := e.delivered(t)
	if staged[0].args.NoticeID != rows[0].id.String() {
		t.Errorf("the staged job names %q rather than the notice that was written (%s)", staged[0].args.NoticeID, rows[0].id)
	}
	if staged[0].args.Workspace != e.e.WS {
		t.Errorf("the staged job carries workspace %s, not this installation's %s", staged[0].args.Workspace, e.e.WS)
	}

	// THE OPTIONS ARE THE LADDER, and this kind's only copy of it. api/jobs.yaml
	// declares notification_email opts_owner: caller, which means the contract
	// publishes no attempt cap and nothing but this enqueue applies one — so an
	// InsertOpts that lost its MaxAttempts would silently put the job on River's
	// own 25-rung default, and an empty Queue would put it on whatever River
	// defaults to rather than where the declaration says these rows land.
	opts := staged[0].opts
	if opts == nil {
		t.Fatal("the mail job was staged with no insert options: River's defaults would decide its queue and its ladder, and the declaration would describe neither")
	}
	if opts.MaxAttempts != notificationMailJobAttempts {
		t.Errorf("the staged job rides %d attempt(s), want the ladder the enqueue owns (%d)",
			opts.MaxAttempts, notificationMailJobAttempts)
	}
	// Read off the contract rather than restated: for a caller-owned kind the
	// declared queue is otherwise only documentation, and this is what makes it
	// true of the rows that actually land.
	spec, declared := jobs.SpecFor(SendNotificationEmailArgs{}.Kind())
	if !declared {
		t.Fatal("api/jobs.yaml declares no notification_email, so this assertion has nothing to hold the enqueue against")
	}
	if opts.Queue != spec.Queue {
		t.Errorf("the staged job lands on queue %q, and api/jobs.yaml declares %q", opts.Queue, spec.Queue)
	}
}

func TestASeatReadingTheirQueueOnScreenStagesNoMailJob(t *testing.T) {
	e := setupNoticeMail(t)
	e.grantRole(t, e.e.AdminUser)
	seatCtx := e.e.As(e.e.AdminUser, nil, integration.AdminPerms)
	if _, err := notices.NewStore(e.db).SaveNotificationPreference(
		seatCtx, notices.ClassApprovalPending, notices.DeliveryInApp); err != nil {
		t.Fatalf("moving the class to the screen: %v", err)
	}

	_, env := e.stageCorrection(t)
	e.deliver(t, env)

	if rows := e.delivered(t); len(rows) != 1 {
		t.Fatalf("the notice itself is still written to the screen; got %d", len(rows))
	}
	if staged := e.queue.staged(); len(staged) != 0 {
		t.Fatalf("a seat who reads their queue on screen had %d mail job(s) staged: %+v", len(staged), staged)
	}
}
