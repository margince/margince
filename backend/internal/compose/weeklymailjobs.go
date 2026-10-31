// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WeeklyMailConfig keeps the internal reporting relay separate from correspondence.
type WeeklyMailConfig struct {
	Mailer        mailer.Mailer
	PublicBaseURL string
}

type weeklyMailCandidate struct {
	review   weekly.Review
	settings identity.Delivery
	words    mailcopy.Copy
	body     emailview.Message
	// namesTeam says the body carries a team's week, which names each member.
	namesTeam bool
}

type weeklySubmission struct {
	attempt  weekly.MailAttempt
	combined routineClaim
	part     morningDigest
	body     emailview.Message
}

func (w *weeklyGenerateWorker) mailWeekly(ctx context.Context, reviewID ids.UUID, now time.Time) {
	if w.mail.Mailer == nil {
		return
	}
	candidate, ready := w.prepareWeeklyMail(ctx, reviewID, now)
	if !ready {
		return
	}
	w.sendWeeklyMail(ctx, reviewID, candidate, now)
}

// sendWeeklyMail claims and submits a prepared weekly mail.
func (w *weeklyGenerateWorker) sendWeeklyMail(ctx context.Context, reviewID ids.UUID, candidate weeklyMailCandidate, now time.Time) {
	submission, claimed, err := w.claimWeeklyMail(ctx, candidate, now)
	if err != nil {
		w.log.WarnContext(ctx, "weekly claim unavailable", "cause", err)
		return
	}
	if !claimed {
		return
	}
	if submission.attempt.Email == "" {
		if routineCombines(candidate.settings) {
			recordRoutineFailure(ctx, w.pool, submission.combined, submission.part, "the seat has no email address", w.log)
		} else {
			w.recordMailFailure(ctx, reviewID, "the seat has no email address")
		}
		return
	}
	if unchanged, err := routineChoicesUnchanged(ctx, w.users, candidate.settings); err != nil || !unchanged {
		if routineCombines(candidate.settings) {
			recordRoutineFailure(ctx, w.pool, submission.combined, submission.part, "delivery choices changed or could not be rechecked", w.log)
		} else {
			w.recordMailFailure(ctx, reviewID, "delivery choices changed or could not be rechecked")
		}
		return
	}
	if reason := w.teamWeekWithdrawn(ctx, candidate); reason != "" {
		if routineCombines(candidate.settings) {
			recordRoutineFailure(ctx, w.pool, submission.combined, submission.part, reason, w.log)
		} else {
			w.recordMailFailure(ctx, reviewID, reason)
		}
		return
	}
	submission.body.AppendContent(routineFooterContent(w.mail.PublicBaseURL, candidate.words))
	bounded, cancel := context.WithTimeout(ctx, mailBudget)
	defer cancel()
	if err := mailer.SendProduct(bounded, w.mail.Mailer, submission.attempt.Email, weeklyMailSubject(candidate), submission.body); err != nil {
		w.log.WarnContext(ctx, "weekly relay submission failed", "review", reviewID, "cause", err)
		if routineCombines(candidate.settings) {
			recordRoutineFailure(ctx, w.pool, submission.combined, submission.part, "relay submission failed", w.log)
		} else {
			w.recordMailFailure(ctx, reviewID, err.Error())
		}
	}
}

func (w *weeklyGenerateWorker) prepareWeeklyMail(ctx context.Context, reviewID ids.UUID, now time.Time) (weeklyMailCandidate, bool) {
	var out weeklyMailCandidate
	if w.optedOutOfWeekly(ctx) {
		return out, false
	}
	settings, err := w.users.MyDelivery(ctx)
	if err != nil {
		w.log.WarnContext(ctx, "weekly preferences unavailable", "cause", err)
		return out, false
	}
	if !w.weeklyDeliveryDue(ctx, settings, now) {
		return out, false
	}
	review, err := currentMailReview(ctx, w.pool, w.engine, now)
	if err != nil || review.ID != reviewID {
		return out, false
	}
	pending, err := w.engine.MailPending(ctx, reviewID)
	if err != nil || !pending {
		return out, false
	}
	words := recipientMailCopy(ctx, w.pool)
	body, namesTeam, err := w.weeklyMessage(ctx, review, settings, now, words)
	if err != nil {
		w.log.WarnContext(ctx, "weekly content unavailable", "cause", err)
		return out, false
	}
	if len(body.Blocks) == 0 {
		return out, false
	}
	return weeklyMailCandidate{review: review, settings: settings, words: words, body: body, namesTeam: namesTeam}, true
}

// teamWeekWithdrawn reads the individual-performance switch again at
// submission. A body naming a team's members was built while it was on, and
// the switch may have gone off since. "" means the mail may go.
func (w *weeklyGenerateWorker) teamWeekWithdrawn(ctx context.Context, candidate weeklyMailCandidate) string {
	if !candidate.namesTeam {
		return ""
	}
	allowed, err := readIndividualPerformance(ctx, w.pool)
	if err != nil {
		w.log.WarnContext(ctx, "individual performance setting unavailable at weekly submission", "cause", err)
		return "the individual performance setting could not be rechecked"
	}
	if !allowed {
		return "individual performance was switched off before the team week was sent"
	}
	return ""
}

func (w *weeklyGenerateWorker) weeklyDeliveryDue(ctx context.Context, settings identity.Delivery, now time.Time) bool {
	window, allowed, err := routineWindowFor(ctx, w.users, now, settings)
	if err != nil {
		w.log.WarnContext(ctx, "weekly schedule unavailable", "cause", err)
		return false
	}
	if !allowed {
		return false
	}
	if settings.WeeklyDay == nil {
		return true
	}
	hours, _, err := w.users.MyWorkingHours(ctx)
	if err != nil {
		w.log.WarnContext(ctx, "weekly working days unavailable", "cause", err)
		return false
	}
	return (int(window.Open.Weekday())+6)%7+1 >= weeklyMailDay(hours, *settings.WeeklyDay)
}

func (w *weeklyGenerateWorker) claimWeeklyMail(ctx context.Context, candidate weeklyMailCandidate, now time.Time) (weeklySubmission, bool, error) {
	out := weeklySubmission{body: candidate.body}
	if !routineCombines(candidate.settings) {
		attempt, claimed, err := w.engine.ClaimMailAttempt(ctx, candidate.review.ID, now)
		out.attempt = attempt
		return out, claimed, err
	}
	part, err := prepareRoutineDigest(ctx, w.pool, mailUser(ctx), now, candidate.settings)
	if err != nil {
		return out, false, err
	}
	window, allowed, err := routineWindowFor(ctx, w.users, now, candidate.settings)
	if err != nil || !allowed {
		return out, false, err
	}
	run, err := routineBriefEngine(w.pool).SnapshotRun(ctx, window.Open)
	if err != nil {
		return out, false, err
	}
	claim, claimed, err := claimRoutine(ctx, w.pool, run.ID, candidate.review.ID, part, now)
	if err != nil || !claimed {
		return out, false, err
	}
	out.attempt, out.combined, out.part = claim.week, claim, part
	if claim.digest {
		out.body.AppendContent(digestContent(part.lines, w.mail.PublicBaseURL, candidate.words))
	}
	return out, true, nil
}

func (w *weeklyGenerateWorker) recordMailFailure(ctx context.Context, reviewID ids.UUID, cause string) {
	if err := w.engine.MailFailed(ctx, reviewID, cause); err != nil {
		w.log.WarnContext(ctx, "weekly failure could not be recorded", "review", reviewID, "cause", err)
	}
}

const mailBudget = 45 * time.Second

func (w *weeklyGenerateWorker) optedOutOfWeekly(ctx context.Context) bool {
	settings, err := w.users.MyDelivery(ctx)
	if err != nil {
		w.log.WarnContext(ctx, "weekly preference unavailable; deferring email", "cause", err)
		return true
	}
	return settings.Weekly != nil && *settings.Weekly == identity.DeliveryNone
}

func weeklyMailSubject(candidate weeklyMailCandidate) string {
	if candidate.settings.ScopeChoice() == identity.RoutineTeam || !weekly.MailUseful(candidate.review) {
		return candidate.body.Title
	}
	return weekly.MailSubject(candidate.review, candidate.words)
}
