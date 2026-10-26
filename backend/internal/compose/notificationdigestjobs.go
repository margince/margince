// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Digest content closes when each recipient’s delivery window opens.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/mailer"
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
	candidates, err := w.notices.DigestCandidates(sysCtx, pass.window)
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
		if err := w.digestFor(sysCtx, wsID, seat, address, pass.window); err != nil {
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
	// window is the local day every claim is filed under, and the instant that
	// day's morning began, which is as late as a notice may be and still be in
	// this message.
	window notices.Window
	// roster maps a colleague who has a morning to where a message reaches
	// them. Absence from it is what skips a seat.
	roster map[ids.UUID]string
}

// morningFor resolves the reporting window and the live recipient roster.
func (w *notificationDigestWorker) morningFor(
	ctx context.Context, now time.Time,
) (digestPass, error) {
	var pass digestPass
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		// Candidate routing includes notices up to this tick; each recipient gets their own cutoff.
		pass.window, err = notificationWindow(ctx, tx, now)
		if err != nil {
			return err
		}

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
	ctx context.Context, wsID ids.UUID, seat ids.UserID, address string, window notices.Window,
) error {
	seatCtx, live, err := seatContext(ctx, w.users, wsID, seat.UUID)
	if err != nil || !live {
		return err
	}
	settings, err := w.users.MyDelivery(seatCtx)
	if err != nil {
		return err
	}
	if routineCombines(settings) && auth.Require(seatCtx, "deal", principal.ActionRead) == nil && (settings.MorningBrief == nil || *settings.MorningBrief != identity.DeliveryNone) {
		return nil
	}
	clock := w.now
	if clock == nil {
		clock = time.Now
	}
	allowed, err := routineMailWindow(seatCtx, w.users, clock(), settings)
	if err != nil || !allowed {
		return err
	}
	personal, _, err := routineWindowFor(seatCtx, w.users, clock(), settings)
	if err != nil {
		return err
	}
	if err := database.WithWorkspaceTx(seatCtx, w.pool, func(tx pgx.Tx) error {
		var err error
		window, err = notificationWindow(seatCtx, tx, personal.Open)
		return err
	}); err != nil {
		return err
	}
	if routineCombines(settings) && auth.Require(seatCtx, "deal", principal.ActionRead) == nil {
		return w.combinedDigestFor(seatCtx, seat, address, window, personal, settings, clock())
	}

	wanted, err := w.notices.DigestStillDue(ctx, seat, window)
	if err != nil {
		return err
	}
	if !wanted {
		// A colleague who moved the class off the batch since the sweep read it,
		// or who has answered everything on screen. The product declining to send
		// rather than failing to.
		return nil
	}
	claimed, err := w.notices.ClaimDigestRun(ctx, seat, window.Day)
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
	w.deliver(ctx, wsID, seat, address, window)
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
	sysCtx context.Context, wsID ids.UUID, seat ids.UserID, address string, window notices.Window,
) {
	// THE RECIPIENT'S OWN AUTHORITY, bound before a single word of content is
	// read. Everything below this line runs as the colleague the message is
	// addressed to.
	seatCtx, live, err := seatContext(sysCtx, w.users, wsID, seat.UUID)
	if err != nil {
		w.log.WarnContext(sysCtx, "the morning digest was not sent: the recipient's authority did not resolve",
			"user", seat, "cause", err)
		w.recordFailure(sysCtx, seat, window.Day, err.Error())
		return
	}
	if !live {
		// The seat was archived after the sweep listed it. A SKIP and not a
		// failure: the claim is spent, and there is nobody left to tell.
		return
	}
	// The claim committed in digestFor, so the reservation is the hold alone.
	lines, ready, err := reserveSend(seatCtx, w.pool, w.digestReading(seatCtx, seat, window), reserveInOwnTx[digestLines](seatCtx, w.pool))
	if err != nil {
		w.log.WarnContext(sysCtx, "the morning digest was not sent: its content could not be read or changed before the send",
			"user", seat, "cause", err)
		w.recordFailure(sysCtx, seat, window.Day, err.Error())
		return
	}
	if !ready {
		// Everything the sweep found has been answered on screen since. A SKIP
		// and not a failure: the claim is spent, and there is nothing left to
		// tell this colleague.
		return
	}

	words := recipientMailCopy(seatCtx, w.pool)

	bounded, cancel := context.WithTimeout(sysCtx, notificationDigestBudget)
	defer cancel()
	if err := mailer.SendProduct(bounded, w.mail.Mailer, address, digestSubject(lines, words), digestMail(lines, w.mail.PublicBaseURL, words)); err != nil {
		// LOGGED AND RECORDED, never retried. SMTP returns no receipt, so a
		// second attempt could not tell a refused message from a delivered one.
		w.log.WarnContext(sysCtx, "the morning digest was attempted and did not go out",
			"user", seat, "cause", err)
		w.recordFailure(sysCtx, seat, window.Day, err.Error())
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

func (w *notificationDigestWorker) combinedDigestFor(seatCtx context.Context, seat ids.UserID, address string, window notices.Window, personal routineWindow, settings identity.Delivery, now time.Time) error {
	worker := briefGenerateWorker{pool: w.pool, users: w.users, log: w.log, mail: BriefMailConfig(w.mail)}
	if (settings.Weekly == nil || *settings.Weekly != identity.DeliveryNone) && worker.sendReadyWeekly(seatCtx, now) {
		return nil
	}
	run, err := routineBriefEngine(w.pool).SnapshotRun(seatCtx, personal.Open)
	if err != nil {
		return err
	}
	var claim routineClaim
	lines, won, err := reserveSend(seatCtx, w.pool, w.digestReading(seatCtx, seat, window), func(lines digestLines, hold func(pgx.Tx) error) (bool, error) {
		var won bool
		var err error
		claim, won, err = claimRoutine(seatCtx, w.pool, run.ID, ids.Nil, morningDigest{lines: lines, window: window}, now, hold)
		return won, err
	})
	if err != nil || !won || !claim.digest {
		return err
	}
	part := morningDigest{lines: lines, window: window}
	if unchanged, err := routineChoicesUnchanged(seatCtx, w.users, settings); err != nil || !unchanged {
		recordRoutineFailure(seatCtx, w.pool, claim, part, "delivery choices changed or could not be rechecked", w.log)
		return nil
	}
	words := recipientMailCopy(seatCtx, w.pool)
	bounded, cancel := context.WithTimeout(seatCtx, notificationDigestBudget)
	defer cancel()
	if err := mailer.SendProduct(bounded, w.mail.Mailer, address, digestSubject(lines, words), digestMail(lines, w.mail.PublicBaseURL, words)); err != nil {
		recordRoutineFailure(seatCtx, w.pool, claim, part, "relay submission failed", w.log)
	}
	return nil
}

// digestReading is how a digest reads its lines for a send: as the seat, with
// the decisions it quotes as its basis.
func (w *notificationDigestWorker) digestReading(seatCtx context.Context, seat ids.UserID, window notices.Window) sendReading[digestLines] {
	return sendReading[digestLines]{
		seat:  seat.UUID,
		clock: func() time.Time { return mailNow(w.now) },
		render: func(excluded map[ids.UUID]bool) (digestLines, sendRefs, bool, error) {
			lines, err := w.readDigestLines(seatCtx, seat, window)
			if err != nil {
				return digestLines{}, sendRefs{}, false, err
			}
			lines = lines.without(excluded)
			return lines, digestRefs(lines), lines.total > 0, nil
		},
		same: sameDigest,
	}
}
