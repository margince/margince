// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The weekly retrospective's mail, at most once.
//
// Its own file rather than another rung in weeklyjobs.go, because it is a
// different obligation. The measurement pass may run as often as the schedule
// likes — the counts converge on one review and the sentence is a correction.
// The mail may not: a message is not idempotent once a relay has taken it, so
// this lane is the one place in the weekly arc where running twice is a defect
// rather than a no-op.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WeeklyMailConfig is the weekly retrospective's outbound channel.
//
// BOTH halves are optional and the lane needs the relay. An installation that
// configured no operator mail mails no weeklies, which is the honest posture
// rather than a boot error — the review is on the screen either way, and the
// mail is a nudge toward it.
type WeeklyMailConfig struct {
	// Mailer is the operator relay. Nil turns the lane off by omission.
	Mailer mailer.Mailer
	// PublicBaseURL is the installation's own canonical origin, for the one
	// link the message carries. Empty omits the link rather than mailing an
	// unusable one built on an empty base.
	PublicBaseURL string
}

// mailWeekly hands one rep's review to the relay, at most once ever.
//
// It returns nothing, for the same reason narrate does: a rep who does not get
// the mail still has the whole review on Home, so a relay outage must not fail
// a job that measured a team's week. What it must not do is send twice, and
// that is the claim's job rather than this function's.
func (w *weeklyGenerateWorkspaceWorker) mailWeekly(
	ctx context.Context, reviewID ids.UUID, now time.Time,
) {
	if w.mail.Mailer == nil {
		// No relay configured. Not an error and not worth a line every Monday.
		return
	}
	// THE CLAIM FIRST, always. Everything after this point is allowed to fail
	// and lose the message; nothing after this point is allowed to produce a
	// second one. See Engine.ClaimMailAttempt for why the trade goes this way,
	// and why turning this into a retry is the wrong repair.
	attempt, claimed, err := w.engine.ClaimMailAttempt(ctx, reviewID, now)
	if err != nil {
		w.log.WarnContext(ctx, "the weekly mail was not attempted: the claim failed",
			"review", reviewID, "cause", err)
		return
	}
	if !claimed {
		// A previous pass already spent this week's one attempt.
		return
	}
	if attempt.Email == "" {
		w.recordMailFailure(ctx, reviewID, "the seat has no email address")
		return
	}

	// The installation's own language, resolved once per message. A rep reads
	// their Home panel in it and then this summary of the same numbers, so an
	// English message to a German installation is the product changing language
	// on its way out of the browser.
	//
	// BaseLanguageForPrompt answers English on any failure and logs it, which is
	// the right trade here too: a week's summary in the wrong language is worth
	// more than no summary, and the claim above is already spent.
	words := mailcopy.For(identity.BaseLanguageForPrompt(ctx, w.pool))

	// Bounded like the narration is, and for the same reason: the workspace
	// job's deadline belongs to every rep in it, and one unreachable relay
	// must not be able to spend it on the first.
	bounded, cancel := context.WithTimeout(ctx, mailBudget)
	defer cancel()
	if err := w.mail.Mailer.Send(bounded, attempt.Email,
		weekly.MailSubject(attempt.Review, words),
		weekly.MailBody(attempt.Review, w.mail.PublicBaseURL, words)); err != nil {
		// LOGGED AND RECORDED, never retried. The attempt is spent; what is
		// left to do is make the absence answerable.
		w.log.WarnContext(ctx, "the weekly mail was attempted and did not go out",
			"review", reviewID, "cause", err)
		w.recordMailFailure(ctx, reviewID, err.Error())
	}
}

// recordMailFailure writes the cause onto the claimed row.
//
// Separate so the failure path has one spelling: the two callers above differ
// only in what went wrong, and a second copy of the log-and-store pair is how
// one of them comes to store nothing.
func (w *weeklyGenerateWorkspaceWorker) recordMailFailure(ctx context.Context, reviewID ids.UUID, cause string) {
	if err := w.engine.MailFailed(ctx, reviewID, cause); err != nil {
		w.log.WarnContext(ctx, "the weekly mail's failure could not be recorded",
			"review", reviewID, "cause", err)
	}
}

// mailBudget bounds ONE rep's send.
//
// mailer.SMTP already puts a 30-second deadline on the exchange and 15 on the
// dial, but those are the transport's own and a caller that trusted them would
// be trusting a number in another package to stay small. This is the weekly
// job's ceiling on the same exchange, stated where the job's deadline is being
// spent.
const mailBudget = 45 * time.Second
