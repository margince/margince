// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one notice that leaves the product: a decision waiting on a colleague
// who asked for that class by mail.
//
// Its own file rather than a rung in approvalnotify.go, for the reason the two
// digests have one: writing the notice is idempotent — the table's dedupe key
// makes a redelivery free — and mailing is not, because a message is not
// idempotent once a relay has taken it. So this is the one place in the
// approval arc where running twice is a defect rather than a no-op, and the
// claim underneath it is the whole of the answer.
//
// It is SEAT MAIL. The recipient is a colleague who works here, being told
// about their own queue, so it goes straight to the relay the way the morning
// brief does rather than through the consent lane that authorizes
// correspondence with a data subject (backend/gates/directmailbypass_test.go
// carries the ratification and the reason).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NotificationMailConfig is the immediate notice's outbound channel.
//
// STRUCTURALLY IDENTICAL TO WeeklyMailConfig and BriefMailConfig, and the
// worker binary converts one to the others rather than resolving the relay
// three times: an operator configures outbound mail once, and three resolutions
// could disagree about whether this installation can send at all. A field added
// to any of them breaks that conversion at compile time, which is the intended
// way to be told.
type NotificationMailConfig struct {
	// Mailer is the operator relay. Nil turns the lane off by omission: a
	// picked-up job spends no claim and sends nothing, so an installation that
	// configures mail later still reaches the notices written after it did.
	Mailer mailer.Mailer
	// PublicBaseURL is the installation's own canonical origin, for the one
	// link the message carries. Empty omits the link rather than mailing an
	// unusable one built on an empty base.
	PublicBaseURL string
}

// SendNotificationEmailArgs mails ONE recorded notice. The workspace travels
// with it because a job carries no session and every read below is
// workspace-predicated; the notice id is the whole of what is to be sent,
// because River persists args verbatim in a table with no workspace column and
// no row-level security — a job names a row and the worker reads it.
type SendNotificationEmailArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	NoticeID  string   `json:"notice_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (SendNotificationEmailArgs) Kind() string { return "notification_email" }

// WorkspaceID binds this message to its tenant (jobs.WorkspaceScoped).
func (a SendNotificationEmailArgs) WorkspaceID() ids.UUID { return a.Workspace }

// notificationMailJobAttempts is the ladder one notice's mail rides, and the
// enqueue site owns it because api/jobs.yaml declares this kind caller-owned.
//
// The rungs are for reaching the WORKER, not for retrying the send: the claim
// is spent before the relay is dialled, so an attempt that got that far sends
// once whatever River does afterwards. What they ride out is the job failing
// before the claim — a pool blip, a rollout mid-flight — and ten rungs on
// River's attempt⁴ backoff span about five hours, which is longer than a
// decision is worth announcing.
const notificationMailJobAttempts = 10

// notificationMailBudget bounds ONE colleague's send.
//
// mailer.SMTP already puts its own deadline on the exchange, but that is the
// transport's and a caller trusting it would be trusting a number in another
// package to stay small. This is the job's ceiling on the same exchange, stated
// where the job's deadline is being spent — and it sits well inside the
// two-minute wall clock api/jobs.yaml gives the kind.
const notificationMailBudget = 45 * time.Second

// notificationMailOpts is the enqueue policy for one notice's message.
//
// No uniqueness window. The claim is what makes a second job harmless — it
// finds the attempt spent and sends nothing — and a unique-by-args window would
// add a second, weaker guard in front of the one that actually holds, in the
// place a reader would then look for the guarantee.
func notificationMailOpts() *river.InsertOpts {
	return &river.InsertOpts{Queue: river.QueueDefault, MaxAttempts: notificationMailJobAttempts}
}

// notificationMailWorker mails one claimed notice.
type notificationMailWorker struct {
	pool    *pgxpool.Pool
	db      *database.DB
	notices *notices.Store
	mail    NotificationMailConfig
	log     *slog.Logger
}

func newNotificationMailWorker(pool *pgxpool.Pool, mail NotificationMailConfig, log *slog.Logger) *notificationMailWorker {
	db := InstallationDB(pool)
	return &notificationMailWorker{
		pool: pool, db: db, notices: notices.NewStore(db), mail: mail, log: log,
	}
}

// notificationMailActor names this sender in the audit rows the claim causes. A
// stamp on somebody's notice has to say what put it there, and the answer is
// never the reader themselves.
const notificationMailActor = "system:notification-mail"

// addNotificationMailJobs registers the sender, UNCONDITIONALLY.
//
// Every other mail lane is a pass this role also schedules, so a role with no
// relay can simply not wire it. This one is staged by the approval-notify
// consumer, which has no view of any role's relay — so a worker gated on the
// config would leave those rows queued behind a job nobody works.
//
// WHAT A RELAY-LESS ROLE THEN DOES IS FINISH THE JOB, and the message is not
// sent by anybody: River hands each row to one worker, and a completed row is
// not offered again. The fallback is the one the config field states — the
// decision is on the reader's Worklist either way — and it is stated here
// rather than left to be inferred, because each role reads its own config, so
// one unconfigured worker beside an armed one is a deployment somebody can
// actually assemble. Work says so at Warn when it happens.
func addNotificationMailJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) {
	addDeclaredWorker[SendNotificationEmailArgs](reg, newNotificationMailWorker(pool, cfg.NotificationMail, log))
}

// Work sends one notice, at most once ever.
//
// WHAT IT RETURNS is decided by the claim. Above it, a failure is returned and
// River retries — nothing was spent, and a later attempt can still deliver.
// Below it, a failure is recorded on the row and the job finishes green: the
// attempt is gone either way, and a retry could only produce a second message.
// api/jobs.yaml carries that second half as this kind's fault declaration.
func (w *notificationMailWorker) Work(ctx context.Context, job *river.Job[SendNotificationEmailArgs]) error {
	noticeID, err := ids.Parse(job.Args.NoticeID)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("notification_email: notice id: %w", err))
	}
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if w.mail.Mailer == nil {
		// No relay on THIS role, and the job ends here: nothing is claimed, so
		// the row keeps its unspent attempt, but River does not offer a
		// completed job again and no other role will send it. Said out loud
		// because it is invisible otherwise — an installation that configured
		// no operator mail sees one line per decision and knows why, and an
		// operator who armed one worker and not its neighbour learns it from
		// this rather than from a colleague asking where the mail went.
		w.log.WarnContext(wsCtx, "a decision was not mailed: this worker has no operator relay configured",
			"notice", noticeID)
		return nil
	}
	sysCtx := principal.WithCorrelationID(wsCtx, ids.NewV7())
	sysCtx = principal.WithActor(sysCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: notificationMailActor,
	})

	// THE CLAIM FIRST, always. Everything after this point is allowed to fail
	// and lose the message; nothing after this point is allowed to produce a
	// second one.
	attempt, claimed, err := w.notices.ClaimEmailAttempt(sysCtx, noticeID)
	if err != nil {
		// RETURNED, not recorded: nothing was claimed, so a later attempt is
		// free to try again — and this is the one failure in the arc that a
		// retry can actually repair. It is also the reason the kind carries a
		// ladder at all.
		return jobs.FaultContext(sysCtx, fmt.Errorf("notification_email: claiming notice %s: %w", noticeID, err))
	}
	if !claimed {
		// Already spent, already answered on screen, or gone. All three mean
		// the same thing here: do not send.
		return nil
	}
	w.deliver(sysCtx, attempt)
	return nil
}

// deliver renders one claimed notice and hands it to the relay.
//
// Split from Work because Work is about the JOB — parsing it, binding it,
// claiming — and this is about the message. The claim is already spent when
// this is called, which is the one thing a reader has to hold while reading it.
func (w *notificationMailWorker) deliver(ctx context.Context, attempt notices.EmailAttempt) {
	address, wanted, err := w.recipient(ctx, attempt)
	if err != nil {
		w.log.WarnContext(ctx, "the notice was not mailed: its recipient could not be resolved",
			"notice", attempt.Notice.ID, "cause", err)
		w.recordMailFailure(ctx, attempt.Notice.ID, err.Error())
		return
	}
	if !wanted || address == "" {
		// A SKIP rather than a failure, and the two are kept apart deliberately.
		// A seat who moved the class to the screen, or one who has left, is the
		// product declining to send rather than failing to — and recording a
		// cause would bury the relay failures that column exists for.
		return
	}

	// The installation's own language, not the reader's. A colleague reads
	// their Worklist in it and then this line about the same card, so an
	// English message to a German installation is the product changing language
	// on its way out of the browser. BaseLanguageForPrompt answers English on
	// any failure and logs it, which is the right trade here too: a message in
	// the wrong language beats no message, and the claim is already spent.
	words := mailcopy.For(identity.BaseLanguageForPrompt(ctx, w.pool))

	// Bounded like the two digests' sends are: the wall clock the kind declares
	// covers the whole job, and one unreachable relay must not spend all of it
	// holding a River worker.
	bounded, cancel := context.WithTimeout(ctx, notificationMailBudget)
	defer cancel()
	if err := w.mail.Mailer.Send(bounded, address,
		notificationMailSubject(attempt.Notice, words),
		notificationMailBody(attempt.Notice, w.mail.PublicBaseURL, words)); err != nil {
		// LOGGED AND RECORDED, never retried. The attempt is spent; what is
		// left to do is make the absence answerable.
		w.log.WarnContext(ctx, "the notice was attempted and did not go out",
			"notice", attempt.Notice.ID, "cause", err)
		w.recordMailFailure(ctx, attempt.Notice.ID, err.Error())
	}
}

// recipient answers where this notice goes, and whether it should go at all.
//
// ONE transaction for both questions, because they are one snapshot of the same
// seat: the preference they hold now, and the address they hold now. Asked
// separately, a seat could answer the first and be gone by the second.
//
// THE PREFERENCE IS RE-READ, and the answer is only ever narrowing. The gate
// that matters ran where the notice was written — only a seat whose class is on
// email has a job staged at all — so this covers the window between that
// transaction and this one. A colleague who changed their mind inside it is
// entitled to the answer they gave last.
func (w *notificationMailWorker) recipient(
	ctx context.Context, attempt notices.EmailAttempt,
) (address string, wanted bool, err error) {
	class, err := notices.ClassFor(attempt.Notice.Kind)
	if err != nil {
		return "", false, err
	}
	err = w.db.Tx(ctx, func(tx pgx.Tx) error {
		delivery, txErr := notices.DeliveryFor(ctx, tx, attempt.Recipient, class)
		if txErr != nil {
			return txErr
		}
		if delivery != notices.DeliveryEmail {
			return nil
		}
		wanted = true
		// LIVE MEMBERSHIP, both halves. Deactivating a seat leaves archived_at
		// NULL, so archived_at alone would mail a departed colleague about a
		// decision they can no longer take. No row means no address, which the
		// caller reads as a skip.
		switch scanErr := tx.QueryRow(ctx,
			`SELECT email FROM app_user WHERE id = $1 AND `+identity.LiveMemberSQL(""),
			attempt.Recipient).Scan(&address); {
		case errors.Is(scanErr, pgx.ErrNoRows):
			address = ""
			return nil
		case scanErr != nil:
			return fmt.Errorf("reading the recipient: %w", scanErr)
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return address, wanted, nil
}

// recordMailFailure writes the cause onto the claimed row.
//
// Separate so the failure path has one spelling: the two callers above differ
// only in what went wrong, and a second copy of the log-and-store pair is how
// one of them comes to store nothing.
func (w *notificationMailWorker) recordMailFailure(ctx context.Context, notice ids.UUID, cause string) {
	if err := w.notices.EmailFailed(ctx, notice, cause); err != nil {
		w.log.WarnContext(ctx, "the notice's failure could not be recorded",
			"notice", notice, "cause", err)
	}
}

// notificationMailSubject is what a colleague sees in their list: the prefix
// that says somebody is waiting, then the notice's own line.
func notificationMailSubject(notice notices.Notice, words mailcopy.Copy) string {
	return words.NotificationSubject + mailcopy.OneLine(notice.Subject)
}

// notificationMailBody is the message itself: why it arrived, what is waiting,
// and where to answer it.
//
// EVERY interpolated value goes through OneLine. mailer.Send refuses line
// breaks in the recipient and the subject, which are the header fields; the
// body is the sender's to keep honest, and a notice's subject and body are
// written by producers that bound their length and not their structure.
func notificationMailBody(notice notices.Notice, base string, words mailcopy.Copy) string {
	var b strings.Builder
	b.WriteString(words.NotificationIntro + "\n\n")
	b.WriteString(mailcopy.OneLine(notice.Subject) + "\n")
	if notice.Body != "" {
		b.WriteString(mailcopy.OneLine(notice.Body) + "\n")
	}
	mailcopy.Link(&b, base, mailcopy.WorklistFragment, words.NotificationOpen)
	return b.String()
}
