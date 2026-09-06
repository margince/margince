// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Asking what a week taught, and storing the answer.
//
// Split from the weekly job beside it because it is a different obligation: the
// job decides WHICH weeks are measured and mails them, and this decides whether
// a week can support a lesson at all. The floor check is the whole reason this
// has to be its own concept — a model asked to find lessons in two rows will
// find them, so the refusal happens before the call rather than after the reply.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// learnBudget bounds ONE rep's learnings.
//
// Longer than the sentence beside it because the work is different in kind: a
// narrative restates counts the model was handed, and a learning has to read
// across deals and commitments to find what they have in common. Still bounded
// for narrateBudget's reason — one wedged provider must not spend the
// workspace's whole deadline on the first rep.
const learnBudget = 60 * time.Second

// learn asks the model what the week taught and stores it, or stores that
// there was too little to say.
//
// It returns nothing, exactly as narrate does: every failure here is a week
// that reads without its lessons, which is a complete review.
//
// THE FLOOR IS CHECKED BEFORE THE CALL. A model asked to find lessons in two
// rows will find them — that is what it is for — so a thin week never becomes a
// request, and the week is stamped insufficient_evidence without a bill.
func (w *weeklyGenerateWorker) learn(ctx context.Context, review weekly.Review, now time.Time) {
	if w.learner == nil {
		// No lane in this role. The deterministic review is the product either
		// way, and the screen says nobody looked.
		return
	}
	in := learningsInput(review)
	if !learnings.Floor(in) {
		// Too thin to learn from. Stamped rather than left unread, so the rep
		// is told the week was looked at and had too little in it.
		if _, err := w.engine.RecordLearnings(ctx, review.ID, nil, now); err != nil {
			w.log.WarnContext(ctx, "the weekly review's verdict could not be stored",
				"week", review.LocalWeekStart.Format(time.DateOnly), "cause", err)
		}
		return
	}

	lang := identity.BaseLanguageForPrompt(ctx, w.pool)
	bounded, cancel := context.WithTimeout(ctx, learnBudget)
	defer cancel()
	reply, err := ai.Ask(bounded, w.learner, learnings.Request(in, lang), func(text string) error {
		_, err := learnings.Parse(text, in)
		return err
	})
	if err != nil {
		w.log.WarnContext(ctx, "the weekly review has no learnings: the model call did not land",
			"week", review.LocalWeekStart.Format(time.DateOnly), "cause", err)
		return
	}
	items, err := learnings.Parse(reply.Text, in)
	if err != nil {
		// A REFUSED reply leaves the week not_run, not insufficient_evidence.
		// The pass did not decide the week held no lesson; it produced
		// something ungrounded, and a later tick may do better.
		w.log.WarnContext(ctx, "the weekly review has no learnings: the reply was refused",
			"week", review.LocalWeekStart.Format(time.DateOnly), "cause", err)
		return
	}
	// The STORE runs on the caller's context, not the bounded one: the model
	// call is what needs a leash, and a write cancelled by the model's own
	// deadline would lose learnings the model successfully produced.
	if _, err := w.engine.RecordLearnings(ctx, review.ID, items, now); err != nil {
		w.log.WarnContext(ctx, "the weekly review's learnings could not be stored",
			"week", review.LocalWeekStart.Format(time.DateOnly), "cause", err)
	}
}

// learningsInput is the week as the learnings prompt reads it.
//
// The deal lines carry their FROZEN ids and labels — the same rows the review
// already shows — so a citation the model returns names something the reader
// can open.
func learningsInput(review weekly.Review) learnings.Input {
	in := learnings.Input{
		WeekStart: review.LocalWeekStart.Format(time.DateOnly),
		Counts: learnings.Counts{
			TasksDue: review.Counts.TasksDue, TasksDone: review.Counts.TasksDone,
			TasksCarriedOver: review.Counts.TasksCarriedOver,
			DealsMoved:       review.Counts.DealsMoved,
			DealsWon:         review.Counts.DealsWon, DealsLost: review.Counts.DealsLost,
			CommitmentsDue:  review.Counts.CommitmentsDue,
			CommitmentsKept: review.Counts.CommitmentsKept,
			MeetingsHeld:    review.Counts.MeetingsHeld,
			LeadsRouted:     review.Counts.LeadsRouted,
		},
	}
	for _, line := range review.Deals {
		in.Deals = append(in.Deals, learnings.Subject{
			Type: learnings.SubjectDeal, ID: line.DealID,
			Label: line.Label, Outcome: line.Outcome,
		})
	}
	return in
}
