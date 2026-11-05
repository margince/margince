// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The actionable brief lane requires an open deal the reader can still resolve.

import (
	"context"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionBriefing binds the briefing lane to the same engine entry point Home
// and the agent tool read, so all three read one queue rather than three
// readings of it.
type attentionBriefing struct {
	engine *briefs.BriefEngine
	// figures is the same reader the feed's deal-figures pass is bound to, and
	// that sameness is the point: this lane keeps an entry exactly when that
	// pass can state the deal's figures. A bespoke visibility probe here would
	// be a second answer to "can this reader resolve this deal", and a row one
	// of them keeps and the other blanks is the defect this closes.
	figures attention.DealFacts
	now     attention.Clock
}

// Queue returns the reader's unanswered entries and the stored run's cutoff.
// Closed, archived and unreadable deals leave this lane; the run remains reviewable.
func (a attentionBriefing) Queue(ctx context.Context) ([]attention.BriefEntry, bool, time.Time, error) {
	// This read WRITES — it resurfaces expired snoozes and records the open —
	// and under a composed read those now ride the page's own transaction. That
	// is the right fate for both: a brief whose page never rendered was not
	// opened. Its errors already fail the page, so it needs no detachment.
	run, err := a.engine.LatestRun(ctx, a.now())
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, time.Time{}, nil
	}
	if err != nil {
		return nil, false, time.Time{}, err
	}
	// Whether this run could read the relationship at all. A run that could not
	// scores every deal's warmth zero, and a zero nobody measured must not be
	// read as a cold relationship — see SignalOf.
	warmthKnown := true
	for _, factor := range run.FactorsOmitted {
		if factor == string(crmcontracts.MorningBriefFactorsOmittedWarmth) {
			warmthKnown = false
		}
	}
	unanswered := make([]attention.BriefEntry, 0, len(run.Items))
	named := make([]ids.UUID, 0, len(run.Items))
	for _, item := range run.Items {
		if !briefs.Unanswered(item) {
			continue
		}
		unanswered = append(unanswered, attention.BriefEntry{
			ID: item.ID, DealID: item.DealID, Rank: item.Rank,
			Composite: item.Composite, Finding: item.Finding,
			// Derived from the persisted vector rather than stored beside it:
			// the vector IS the reason, so a second column could disagree with
			// the numbers a reader can already see decomposed on the card.
			Signal: string(briefs.SignalOf(item.Features, warmthKnown)),
		})
		named = append(named, item.DealID)
	}
	resolvable, err := a.figures.Figures(ctx, named)
	if err != nil {
		return nil, false, time.Time{}, err
	}
	entries := make([]attention.BriefEntry, 0, len(unanswered))
	for _, entry := range unanswered {
		if figures, ok := resolvable[entry.DealID]; ok && !figures.Closed {
			entries = append(entries, entry)
		}
	}
	return entries, true, run.AsOf, nil
}
