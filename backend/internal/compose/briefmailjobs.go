// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BriefMailConfig shares relay configuration with the weekly reporting lane.
type BriefMailConfig struct {
	Mailer        mailer.Mailer
	PublicBaseURL string
}

type morningMailCandidate struct {
	run      briefs.BriefRun
	settings identity.Delivery
	words    mailcopy.Copy
	body     string
	digest   morningDigest
}

func (w *briefGenerateWorker) mailMorning(ctx context.Context, observed briefs.BriefRun, now time.Time) {
	if w.mail.Mailer == nil || observed.UserID != mailUser(ctx) {
		return
	}
	candidate, ready, err := w.prepareMorningMail(ctx, now)
	if err != nil {
		w.log.WarnContext(ctx, "morning content unavailable; deferring email", "cause", err)
		return
	}
	if !ready {
		return
	}
	attempt, combined, claimed, err := w.claimMorningMail(ctx, candidate, now)
	if err != nil {
		w.log.WarnContext(ctx, "morning claim unavailable", "cause", err)
		return
	}
	if !claimed {
		return
	}
	body := candidate.body
	if combined.digest {
		body += "\n\n" + digestMessage(candidate.digest.lines, w.mail.PublicBaseURL, candidate.words)
	}
	body += routineFooter(w.mail.PublicBaseURL, candidate.words)
	if attempt.Email == "" {
		if routineCombines(candidate.settings) {
			recordRoutineFailure(ctx, w.pool, combined, candidate.digest, "the seat has no email address", w.log)
		} else {
			w.recordMailFailure(ctx, candidate.run.ID, "the seat has no email address")
		}
		return
	}
	if unchanged, err := routineChoicesUnchanged(ctx, w.users, candidate.settings); err != nil || !unchanged {
		if routineCombines(candidate.settings) {
			recordRoutineFailure(ctx, w.pool, combined, candidate.digest, "delivery choices changed or could not be rechecked", w.log)
		} else {
			w.recordMailFailure(ctx, candidate.run.ID, "delivery choices changed or could not be rechecked")
		}
		return
	}
	bounded, cancel := context.WithTimeout(ctx, briefMailBudget)
	defer cancel()
	if err := mailer.SendLocalized(bounded, w.mail.Mailer, attempt.Email, briefs.MailSubject(candidate.run, candidate.words), body, string(candidate.words.Language)); err != nil {
		w.log.WarnContext(ctx, "morning relay submission failed", "run", candidate.run.ID, "cause", err)
		if routineCombines(candidate.settings) {
			recordRoutineFailure(ctx, w.pool, combined, candidate.digest, "relay submission failed", w.log)
		} else {
			w.recordMailFailure(ctx, candidate.run.ID, err.Error())
		}
	}
}

func (w *briefGenerateWorker) prepareMorningMail(ctx context.Context, now time.Time) (morningMailCandidate, bool, error) {
	var out morningMailCandidate
	settings, err := w.users.MyDelivery(ctx)
	if err != nil {
		return out, false, err
	}
	if settings.MorningBrief != nil && *settings.MorningBrief == identity.DeliveryNone {
		return out, false, nil
	}
	window, allowed, err := w.morningWindow(ctx, now, settings)
	if err != nil || !allowed {
		return out, false, err
	}
	if routineCombines(settings) && (settings.Weekly == nil || *settings.Weekly != identity.DeliveryNone) && w.sendReadyWeekly(ctx, now) {
		return out, false, nil
	}
	run, err := w.engine.SnapshotRun(ctx, window.Open)
	if err != nil {
		return out, false, err
	}
	run.LocalDay = window.Day
	words := mailcopy.For(identity.BaseLanguageForPrompt(ctx, w.pool))
	body, useful, err := w.morningMessage(ctx, run, now, words)
	if err != nil {
		return out, false, err
	}
	part, err := prepareRoutineDigest(ctx, w.pool, run.UserID, now, settings)
	if err != nil {
		return out, false, err
	}
	if !morningEditionWanted(settings, body, useful, part.lines.total) {
		return out, false, nil
	}
	return morningMailCandidate{run: run, settings: settings, words: words, body: body, digest: part}, true, nil
}

func (w *briefGenerateWorker) claimMorningMail(ctx context.Context, candidate morningMailCandidate, now time.Time) (briefs.MailAttempt, routineClaim, bool, error) {
	if routineCombines(candidate.settings) {
		claim, claimed, err := claimRoutine(ctx, w.pool, candidate.run.ID, ids.Nil, candidate.digest, now)
		return claim.morning, claim, claimed, err
	}
	attempt, claimed, err := w.engine.ClaimMailAttempt(ctx, candidate.run.ID, now)
	return attempt, routineClaim{}, claimed, err
}

func (w *briefGenerateWorker) recordMailFailure(ctx context.Context, runID ids.UUID, cause string) {
	if err := w.engine.MailFailed(ctx, runID, cause); err != nil {
		w.log.WarnContext(ctx, "morning failure could not be recorded", "run", runID, "cause", err)
	}
}

const briefMailBudget = 45 * time.Second

func morningEditionWanted(settings identity.Delivery, body string, useful bool, notices int) bool {
	if body == "" && notices == 0 {
		return false
	}
	if useful || notices > 0 {
		return true
	}
	return settings.QuietDay != nil && *settings.QuietDay
}
