// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/emailview"
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
	body     emailview.Message
	digest   morningDigest
	basis    sendBasis
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
	w.sendMorning(ctx, candidate, now)
}

// sendMorning reserves the morning's one send against the work its body
// names (sendbasis.go) and hands the message to the relay after that
// reservation commits. No transaction spans the relay call.
func (w *briefGenerateWorker) sendMorning(ctx context.Context, candidate morningMailCandidate, now time.Time) {
	attempt, combined, claimed, err := w.claimMorningMail(ctx, candidate, now)
	if errors.Is(err, errSendBasisChanged) {
		// The work moved between the read and the reservation, which rolled
		// back. Read it once more; a second change waits for the next tick.
		var ready bool
		if candidate, ready, err = w.prepareMorningMail(ctx, now); err != nil || !ready {
			return
		}
		attempt, combined, claimed, err = w.claimMorningMail(ctx, candidate, now)
	}
	if err != nil {
		w.log.WarnContext(ctx, "morning claim unavailable", "cause", err)
		return
	}
	if !claimed {
		return
	}
	fail := func(cause string) {
		if routineCombines(candidate.settings) {
			recordRoutineFailure(ctx, w.pool, combined, candidate.digest, cause, w.log)
		} else {
			w.recordMailFailure(ctx, candidate.run.ID, cause)
		}
	}
	if attempt.Email == "" {
		fail("the seat has no email address")
		return
	}
	if unchanged, err := routineChoicesUnchanged(ctx, w.users, candidate.settings); err != nil || !unchanged {
		fail("delivery choices changed or could not be rechecked")
		return
	}
	body := candidate.body
	if combined.digest {
		body.AppendContent(digestContent(candidate.digest.lines, w.mail.PublicBaseURL, candidate.words))
	}
	body.AppendContent(routineFooterContent(w.mail.PublicBaseURL, candidate.words))
	bounded, cancel := context.WithTimeout(ctx, briefMailBudget)
	defer cancel()
	if err := mailer.SendProduct(bounded, w.mail.Mailer, attempt.Email, morningMailSubject(candidate), body); err != nil {
		w.log.WarnContext(ctx, "morning relay submission failed", "run", candidate.run.ID, "cause", err)
		if routineCombines(candidate.settings) {
			fail("relay submission failed")
		} else {
			fail(err.Error())
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
	words := recipientMailCopy(ctx, w.pool)
	render := func(excluded map[ids.UUID]bool) (morningMailCandidate, sendRefs, bool, error) {
		body, useful, items, err := w.morningMessageItems(ctx, now, words, false, excluded)
		if err != nil {
			return out, sendRefs{}, false, err
		}
		part, err := prepareRoutineDigest(ctx, w.pool, run.UserID, now, settings)
		if err != nil {
			return out, sendRefs{}, false, err
		}
		part.lines = part.lines.without(excluded)
		if body.Title == "" && part.lines.total > 0 {
			body = (emailview.Content{}).Message(briefs.MailSubject(briefs.BriefRun{LocalDay: now.In(window.Day.Location())}, words), string(words.Language), words.DigestPreview)
		}
		wanted := morningEditionWanted(settings, body.Text, useful, part.lines.total)
		return morningMailCandidate{run: run, settings: settings, words: words, body: body, digest: part}, morningRefs(items, part.lines), wanted, nil
	}
	candidate, basis, ready, err := readSendPayload(ctx, w.pool, sendReading[morningMailCandidate]{
		seat: run.UserID, team: settings.TeamID, clock: w.mailClock, render: render, same: sameMorning,
	})
	candidate.basis = basis
	return candidate, ready, err
}

// mailClock is the clock a morning's basis is checked against, read afresh
// at each check.
func (w *briefGenerateWorker) mailClock() time.Time { return mailNow(w.now) }

// sameMorning is whether two readings of the morning say the same words.
func sameMorning(a, b morningMailCandidate) bool {
	return a.body.Title == b.body.Title && a.body.Text == b.body.Text && sameDigest(a.digest.lines, b.digest.lines)
}

// sameDigest is whether two readings of a digest say the same lines.
func sameDigest(a, b digestLines) bool {
	return a.total == b.total && slices.Equal(a.quotable, b.quotable)
}

func (w *briefGenerateWorker) claimMorningMail(ctx context.Context, candidate morningMailCandidate, now time.Time) (briefs.MailAttempt, routineClaim, bool, error) {
	hold := func(tx pgx.Tx) error { return holdSendBasisTx(ctx, tx, candidate.basis, w.mailClock) }
	if routineCombines(candidate.settings) {
		claim, claimed, err := claimRoutine(ctx, w.pool, candidate.run.ID, ids.Nil, candidate.digest, now, hold)
		return claim.morning, claim, claimed, err
	}
	var attempt briefs.MailAttempt
	var claimed bool
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		if attempt, claimed, err = w.engine.ClaimMailAttemptTx(ctx, tx, candidate.run.ID, now); err != nil || !claimed {
			return err
		}
		return hold(tx)
	})
	return attempt, routineClaim{}, claimed && err == nil, err
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

func morningMailSubject(candidate morningMailCandidate) string { return candidate.body.Title }
