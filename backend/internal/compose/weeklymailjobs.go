// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
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
	body     string
}

type weeklySubmission struct {
	attempt  weekly.MailAttempt
	combined routineClaim
	part     morningDigest
	body     string
}

func (w *weeklyGenerateWorker) mailWeekly(ctx context.Context, reviewID ids.UUID, now time.Time) {
	if w.mail.Mailer == nil {
		return
	}
	candidate, ready := w.prepareWeeklyMail(ctx, reviewID, now)
	if !ready {
		return
	}
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
	bounded, cancel := context.WithTimeout(ctx, mailBudget)
	defer cancel()
	if err := mailer.SendLocalized(bounded, w.mail.Mailer, submission.attempt.Email, weekly.MailSubject(candidate.review, candidate.words), submission.body, string(candidate.words.Language)); err != nil {
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
	if !weekly.MailUseful(review) && settings.ScopeChoice() == identity.RoutinePersonal {
		return out, false
	}
	pending, err := w.engine.MailPending(ctx, reviewID)
	if err != nil || !pending {
		return out, false
	}
	words := mailcopy.For(identity.BaseLanguageForPrompt(ctx, w.pool))
	body, err := w.weeklyMessage(ctx, review, settings, now, words)
	if err != nil {
		w.log.WarnContext(ctx, "weekly content unavailable", "cause", err)
		return out, false
	}
	return weeklyMailCandidate{review: review, settings: settings, words: words, body: body}, true
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
		out.body += "\n\n" + digestMessage(part.lines, w.mail.PublicBaseURL, candidate.words)
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
