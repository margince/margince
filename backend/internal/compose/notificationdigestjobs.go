// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The morning digest: one message a day to a colleague who asked for a class of
// notice in a batch rather than one at a time.
//
// HOURLY AND NOT DAILY, for the reason the overnight brief ticks hourly: the
// morning it aims at is the installation's LOCAL one, an hourly tick crosses
// that hour in every zone the setting can name, and the per-(recipient, day)
// claim is what stops twenty-four ticks producing twenty-four messages. The
// same property makes a missed morning self-healing — a worker down until nine
// sends the digest on its next tick rather than skipping the day.
//
// It is SEAT MAIL, on the terms the brief digest and the immediate notice mail
// are: a colleague being told what reached their own queue, by the route they
// chose for it. There is no consent question about telling a colleague what
// their own worklist holds, and routing it through the delivery lane would file
// an internal batch on a customer's timeline
// (backend/gates/directmailbypass_test.go carries the ratification).
//
// WHAT IT MAY SAY is decided in notificationdigestrender.go, under the
// recipient's own authority. Read that file before changing anything here that
// touches the body: this one is about WHEN a message goes and to WHOM.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NotificationDigestArgs runs one morning-digest pass.
type NotificationDigestArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (NotificationDigestArgs) Kind() string { return "notification_digest" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace and walks them itself (jobs.FleetWide, ADR-0103).
func (NotificationDigestArgs) FleetWide() {}

// notificationDigestActor names this sender in the audit rows the claim causes.
// A stamp on somebody's morning has to say what put it there, and the answer is
// never the reader themselves.
const notificationDigestActor = "system:notification-digest"

// notificationDigestBudget bounds ONE colleague's send.
//
// mailer.SMTP already puts its own deadline on the exchange, but that is the
// transport's and a caller trusting it would be trusting a number in another
// package to stay small. This is the pass's ceiling on the same exchange, stated
// where the deadline is being spent — and the workspace's whole tick belongs to
// every seat in it, so one unreachable relay must not spend it on the first.
const notificationDigestBudget = 45 * time.Second

// notificationDigestWorker sends every due seat's morning across the fleet.
type notificationDigestWorker struct {
	pool    *pgxpool.Pool
	notices *notices.Store
	users   *identity.Service
	// mail is the outbound channel, off by omission: a nil Mailer sends no
	// digests and — crucially — claims no mornings, so an installation that
	// configures a relay tomorrow still reaches tomorrow's batch.
	mail NotificationMailConfig
	log  *slog.Logger
	// now is the injected clock (nil = wall clock). The morning is read at
	// execution time and not at enqueue time: a tick that waited in the queue
	// past midnight must send the day it actually runs in, which is the day its
	// reader will open.
	now func() time.Time
}

func newNotificationDigestWorker(
	pool *pgxpool.Pool, mail NotificationMailConfig, log *slog.Logger,
) *notificationDigestWorker {
	return &notificationDigestWorker{
		pool:    pool,
		notices: notices.NewStore(InstallationDB(pool)),
		users:   identity.NewService(pool),
		mail:    mail,
		log:     log,
		now:     time.Now,
	}
}

// addNotificationDigestJobs registers the morning pass, UNCONDITIONALLY.
//
// A role wired without a relay finishes each tick as the no-op it is rather than
// leaving the kind unregistered: an unregistered kind fails jobs.MustBeTotal at
// boot, and gating registration on the relay would make an installation's mail
// configuration decide whether its worker can start.
func addNotificationDigestJobs(
	reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger,
) {
	addDeclaredWorker[NotificationDigestArgs](reg, newNotificationDigestWorker(pool, cfg.NotificationMail, log))
}

// Work fans the pass out over every live workspace (jobs.FleetWide).
func (w *notificationDigestWorker) Work(ctx context.Context, _ *river.Job[NotificationDigestArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.digestOneWorkspace))
}

func (w *notificationDigestWorker) digestOneWorkspace(ctx context.Context, workspace ids.UUID) error {
	clock := w.now
	if clock == nil {
		clock = time.Now
	}
	return w.digestWorkspace(principal.WithWorkspaceID(ctx, workspace), workspace, clock().UTC())
}

// digestWorkspace sends this workspace's mornings, and reports what it could
// not do.
//
// One seat's failure does not cost the others theirs: the loop records the error
// and carries on, then fails the job with all of them joined. A pass that
// stopped at the first failure would leave a whole team without their morning
// because one colleague had a broken authority row, and River's retry would keep
// hitting that seat first.
func (w *notificationDigestWorker) digestWorkspace(ctx context.Context, wsID ids.UUID, now time.Time) error {
	if w.mail.Mailer == nil {
		// No relay configured. Not an error, and not a claim: a morning claimed
		// here would be an attempt nobody ever made.
		return nil
	}
	// The enumeration runs as the SYSTEM: it reads the workspace's mode, the
	// installation timezone, the seat roster and each seat's own routing choice —
	// installation-level facts about who is owed a message, which belong to no
	// colleague. Every read of what a message may SAY happens under its
	// recipient's own principal, in deliver below.
	sysCtx := principal.WithCorrelationID(ctx, ids.NewV7())
	sysCtx = principal.WithActor(sysCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: notificationDigestActor,
	})
	pass, err := w.morningFor(sysCtx, now)
	if err != nil {
		return err
	}
	if !pass.due {
		return nil
	}
	candidates, err := w.notices.DigestCandidates(sysCtx, pass.day)
	if err != nil {
		return err
	}
	var failures []error
	served := 0
	for _, seat := range candidates {
		// THE ROSTER NARROWS THE CANDIDATES, and the two halves are asked in the
		// packages that own them: notices answers who has something waiting,
		// identity's own live-member spelling answers who is a colleague with a
		// morning. A recipient who is not on it — an agent, a read seat, somebody
		// who has left — is skipped without a claim, because a claim spent there
		// is an attempt that could never be made.
		address, hasAMorning := pass.roster[seat.UUID]
		if !hasAMorning || address == "" {
			continue
		}
		served++
		if err := w.digestFor(sysCtx, wsID, seat, address, pass.day); err != nil {
			failures = append(failures, fmt.Errorf("morning digest for user %s: %w", seat, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("notification_digest_workspace: %d failure(s) over %d seat(s): %w",
			len(failures), served, errors.Join(failures...))
	}
	return nil
}

// digestPass is what one workspace tick resolved before it sent anything:
// whether this is the workspace's morning at all, which local day it is, and who
// has a morning here.
type digestPass struct {
	// due is false when this tick is not the workspace's morning — an hour
	// before the briefing hour. Nothing follows.
	due bool
	// day is the installation's local date, which every claim is filed under.
	day time.Time
	// roster maps a colleague who has a morning to where a message reaches
	// them. Absence from it is what skips a seat.
	roster map[ids.UUID]string
}

// morningFor answers which local day this tick belongs to, and whether the
// workspace's morning has arrived at all.
//
// THE FLOOR IS CHECKED BEFORE ANYTHING IS CLAIMED, which is why it lives up
// here rather than inside the per-seat loop: a tick at four in the morning must
// leave every seat's attempt unspent so the five o'clock tick can still send.
//
// briefs.LocalDayAt and briefingHour rather than a second spelling of either:
// this is the same morning the overnight brief aims at, and two answers to
// "which local day is it, and has the morning started" would let one colleague's
// brief and their digest disagree about the day they are both about.
func (w *notificationDigestWorker) morningFor(
	ctx context.Context, now time.Time,
) (digestPass, error) {
	var pass digestPass
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		day, local, err := briefs.LocalDayAt(ctx, tx, now)
		if err != nil {
			return err
		}
		if local.Hour() < briefingHour {
			return nil
		}
		pass.due, pass.day = true, day
		pass.roster, err = seatsWithAMorning(ctx, tx)
		return err
	})
	return pass, err
}

// digestFor sends one colleague's morning, at most once ever.
//
// THE ROUTING CHOICE BEFORE THE CLAIM, and the order is the whole point. The
// claim spends this seat's ONE attempt for the day, ever; asking afterwards
// would burn it on a colleague who had just moved the class back to their
// screen, so the morning they changed their mind on could never be sent.
//
// THE CLAIM BEFORE THE RELAY, for the mirror reason. Everything after it is
// allowed to fail and lose the message; nothing after it is allowed to produce a
// second one.
func (w *notificationDigestWorker) digestFor(
	ctx context.Context, wsID ids.UUID, seat ids.UserID, address string, day time.Time,
) error {
	wanted, err := w.notices.DigestStillDue(ctx, seat, day)
	if err != nil {
		return err
	}
	if !wanted {
		// A colleague who moved the class off the batch since the sweep read it,
		// or who has answered everything on screen. The product declining to send
		// rather than failing to.
		return nil
	}
	claimed, err := w.notices.ClaimDigestRun(ctx, seat, day)
	if err != nil {
		// RETURNED, not recorded: nothing was claimed, so a later tick is free to
		// try again, and this is the one failure in the arc a retry can repair.
		return err
	}
	if !claimed {
		// An earlier tick already spent this day's one attempt. Reachable even
		// though the candidate read anti-joins the claim table: two ticks can
		// both pass that read, and the primary key is what decides between them.
		return nil
	}
	w.deliver(ctx, wsID, seat, address, day)
	return nil
}

// deliver renders one claimed morning and hands it to the relay.
//
// It returns nothing, the way the brief's send does: a colleague who does not
// get the message still has every notice in it on their screen, so a relay
// outage must not fail a pass that served a whole team. What must not happen is
// the failure vanishing — the cause goes onto the claimed row, so a missing
// morning is answerable.
func (w *notificationDigestWorker) deliver(
	sysCtx context.Context, wsID ids.UUID, seat ids.UserID, address string, day time.Time,
) {
	// THE RECIPIENT'S OWN AUTHORITY, bound before a single word of content is
	// read. Everything below this line runs as the colleague the message is
	// addressed to.
	seatCtx, err := seatContext(sysCtx, w.users, wsID, seat.UUID)
	if err != nil {
		w.log.WarnContext(sysCtx, "the morning digest was not sent: the recipient's authority did not resolve",
			"user", seat, "cause", err)
		w.recordFailure(sysCtx, seat, day, err.Error())
		return
	}
	lines, err := w.readDigestLines(seatCtx, seat, day)
	if err != nil {
		w.log.WarnContext(sysCtx, "the morning digest was not sent: its content could not be read",
			"user", seat, "cause", err)
		w.recordFailure(sysCtx, seat, day, err.Error())
		return
	}
	if lines.total == 0 {
		// Everything the sweep found has been answered on screen since. A SKIP
		// and not a failure: the claim is spent, and there is nothing left to
		// tell this colleague.
		return
	}

	// The installation's own language, not the reader's. A colleague reads their
	// Worklist in it and then this summary of the same queue, so an English
	// message to a German installation is the product changing language on its
	// way out of the browser. BaseLanguageForPrompt answers English on any
	// failure and logs it, which is the right trade here too: a morning in the
	// wrong language beats no morning, and the claim is already spent.
	words := mailcopy.For(identity.BaseLanguageForPrompt(sysCtx, w.pool))
	bounded, cancel := context.WithTimeout(sysCtx, notificationDigestBudget)
	defer cancel()
	if err := w.mail.Mailer.Send(bounded, address,
		digestSubject(lines, words),
		digestMessage(lines, w.mail.PublicBaseURL, words)); err != nil {
		// LOGGED AND RECORDED, never retried. SMTP returns no receipt, so a
		// second attempt could not tell a refused message from a delivered one.
		w.log.WarnContext(sysCtx, "the morning digest was attempted and did not go out",
			"user", seat, "cause", err)
		w.recordFailure(sysCtx, seat, day, err.Error())
	}
}

// recordFailure writes the cause onto the claimed row.
//
// Separate so the failure path has one spelling: the three callers above differ
// only in what went wrong, and a second copy of the log-and-store pair is how
// one of them comes to store nothing.
func (w *notificationDigestWorker) recordFailure(
	ctx context.Context, seat ids.UserID, day time.Time, cause string,
) {
	if err := w.notices.DigestFailed(ctx, seat, day, cause); err != nil {
		w.log.WarnContext(ctx, "the morning digest's failure could not be recorded",
			"user", seat, "cause", err)
	}
}
