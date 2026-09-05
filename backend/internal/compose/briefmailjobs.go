// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The morning brief's mail, at most once per rep per local day.
//
// Its own file rather than another rung in briefjobs.go, for the reason the
// weekly's lane has one: the assembly pass may run as often as the schedule
// likes, because a run converges on one row and re-ranking is a correction. The
// mail may not — a message is not idempotent once a relay has taken it — so
// this is the one place in the morning arc where running twice is a defect
// rather than a no-op.

import (
	"context"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BriefMailConfig is the morning brief's outbound channel.
//
// BOTH halves are optional and the lane needs the relay. An installation that
// configured no operator mail mails no briefs, which is the honest posture
// rather than a boot error — the brief is on the screen either way, and the
// mail is a nudge toward it.
//
// STRUCTURALLY IDENTICAL TO WeeklyMailConfig, and the worker converts one to
// the other rather than resolving the relay twice: an operator configures
// outbound mail once, and two resolutions could disagree about whether this
// installation can send at all. A field added to either struct breaks that
// conversion at compile time, which is the intended way to be told.
type BriefMailConfig struct {
	// Mailer is the operator relay. Nil turns the lane off by omission.
	Mailer mailer.Mailer
	// PublicBaseURL is the installation's own canonical origin, for the one
	// link the message carries. Empty omits the link rather than mailing an
	// unusable one built on an empty base.
	PublicBaseURL string
}

// mailMorning hands one rep's run to the relay, at most once ever.
//
// It returns nothing, for the reason the weekly's does: a rep who does not get
// the mail still has the whole brief on Home, so a relay outage must not fail a
// job that assembled a team's morning. What it must not do is send twice, and
// that is the claim's job rather than this function's.
func (w *briefGenerateWorkspaceWorker) mailMorning(
	ctx context.Context, run briefs.BriefRun, now time.Time, localHour int,
) {
	if w.mail.Mailer == nil {
		// No relay configured. Not an error and not worth a line every morning.
		return
	}
	// THE PREFERENCES BEFORE THE CLAIM, and the order is the whole point.
	//
	// The claim spends this rep's ONE attempt for the day, ever. Checking after
	// it would burn that attempt on a rep who asked for no mail — so if they
	// later turned it back on, the day they changed their mind in could never be
	// sent, and nothing would say why.
	if !w.wantsMorningMail(ctx, run, localHour) {
		return
	}
	// THE CLAIM FIRST, always. Everything after this point is allowed to fail
	// and lose the message; nothing after this point is allowed to produce a
	// second one.
	attempt, claimed, err := w.engine.ClaimMailAttempt(ctx, run.ID, now)
	if err != nil {
		w.log.WarnContext(ctx, "the morning brief was not mailed: the claim failed",
			"run", run.ID, "cause", err)
		return
	}
	if !claimed {
		// A previous pass already spent this day's one attempt.
		return
	}
	if attempt.Email == "" {
		w.recordMailFailure(ctx, run.ID, "the seat has no email address")
		return
	}

	// The installation's own language. A rep reads their Home panel in it and
	// then this summary of the same queue, so an English message to a German
	// installation is the product changing language on its way out of the
	// browser. BaseLanguageForPrompt answers English on any failure and logs it,
	// which is the right trade here too: a morning in the wrong language beats
	// no morning, and the claim above is already spent.
	words := mailcopy.For(identity.BaseLanguageForPrompt(ctx, w.pool))

	// Bounded like the weekly's send is: the workspace job's deadline belongs to
	// every rep in it, and one unreachable relay must not spend it on the first.
	bounded, cancel := context.WithTimeout(ctx, briefMailBudget)
	defer cancel()
	if err := w.mail.Mailer.Send(bounded, attempt.Email,
		briefs.MailSubject(attempt.Run, words),
		briefs.MailBody(attempt.Run, w.mail.PublicBaseURL, words)); err != nil {
		// LOGGED AND RECORDED, never retried. The attempt is spent; what is left
		// to do is make the absence answerable.
		w.log.WarnContext(ctx, "the morning brief was attempted and did not go out",
			"run", run.ID, "cause", err)
		w.recordMailFailure(ctx, run.ID, err.Error())
	}
}

// recordMailFailure writes the cause onto the claimed row.
//
// Separate so the failure path has one spelling: the two callers above differ
// only in what went wrong, and a second copy of the log-and-store pair is how
// one of them comes to store nothing.
func (w *briefGenerateWorkspaceWorker) recordMailFailure(
	ctx context.Context, runID ids.UUID, cause string,
) {
	if err := w.engine.MailFailed(ctx, runID, cause); err != nil {
		w.log.WarnContext(ctx, "the morning brief's failure could not be recorded",
			"run", runID, "cause", err)
	}
}

// briefMailBudget bounds ONE rep's send.
//
// mailer.SMTP already puts its own deadline on the exchange, but that is the
// transport's and a caller trusting it would be trusting a number in another
// package to stay small. This is the morning job's ceiling on the same exchange,
// stated where the job's deadline is being spent.
const briefMailBudget = 45 * time.Second

// wantsMorningMail reports whether this rep asked for this morning's message.
//
// TWO questions, and they are different. Whether they want the morning mail at
// all is `morning_brief_delivery`, and it fails OPEN: a settings read that
// errors leaves the mail going out, because a rep who wanted their brief and did
// not get it is worse served than one who gets a message they meant to switch
// off — the second is an annoyance they can fix from the same page, the first is
// silence they have no way to notice.
//
// Whether they want it YET is `delivery_hour_local`, a floor rather than an
// appointment — see the check itself for why answering false there is a skip
// and not a refusal.
//
// Whether they want it on a QUIET morning is `quiet_day_notice`, and that one
// fails CLOSED, because the two defaults answer different questions. "Send me my
// brief" is the installation's default and a rep who never chose gets it. "Tell
// me even when there is nothing" is a thing a person asks for; sending it to
// everyone who never chose would mail the whole company "nothing is waiting on
// you" every quiet morning, and that message teaches its own readers to filter
// the ones that matter.
func (w *briefGenerateWorkspaceWorker) wantsMorningMail(
	ctx context.Context, run briefs.BriefRun, localHour int,
) bool {
	settings, err := w.users.MyDelivery(ctx)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		// NOT a read failure: MyDelivery gates on LiveMemberSQL, so this is the
		// seat having been deactivated or archived between the roster read and
		// now. Failing open here would claim a departed rep's one attempt for
		// the day and then record "the seat has no email address", which is
		// false — the seat has one and is not live. Reinstating them the same
		// morning could then never deliver, and the row would point an operator
		// at the wrong problem.
		return false
	case err != nil:
		w.log.WarnContext(ctx, "the morning delivery preference could not be read; sending anyway",
			"user", run.UserID, "cause", err)
		return true
	}
	if settings.MorningBrief != nil && *settings.MorningBrief == identity.DeliveryNone {
		return false
	}
	// AND NOT BEFORE THE HOUR THEY CHOSE. `delivery_hour_local` is a FLOOR in
	// the installation's reporting zone, not an appointment: the pass ticks
	// hourly, so the message goes out on the first tick at or after the hour,
	// and a rep who asked for nine does not get their morning at six.
	//
	// Answering false here is a SKIP and not a refusal, which is the whole
	// reason it sits above the claim with the other two: the run keeps its
	// unspent attempt, RunsAwaitingMail finds it again, and a later tick sends
	// it. Below the claim this would burn the day's one attempt at six o'clock
	// and the nine o'clock tick would have nothing left to send.
	//
	// A rep who never chose has no floor and is mailed by the pass that
	// assembled them, which is the behaviour every rep had before the setting
	// was honoured at all.
	if settings.HourLocal != nil && localHour < *settings.HourLocal {
		return false
	}
	// briefs.WaitingCount, not a count of its own: the body asks the same
	// question to decide what a quiet morning SAYS, and two spellings would let
	// this lane send a message whose text disagrees with why it was sent.
	if briefs.WaitingCount(run) == 0 {
		return settings.QuietDay != nil && *settings.QuietDay
	}
	return true
}
