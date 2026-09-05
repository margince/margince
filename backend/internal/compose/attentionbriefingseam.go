// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The overnight brief's lane, and the one rule it applies that the engine does
// not: an entry is served only while the reader can still resolve its deal.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
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

// Queue serves the acting rep's unanswered briefing entries for today, and
// whether a run exists at all.
//
// No run for today reads as an EMPTY lane with ran=false, not a refusal.
// LatestRun answers ErrNotFound both when the night has not produced one and
// when a rep is new, and neither is a permission problem — reporting them as
// a withheld lane would tell the rep something was hidden from her when
// nothing was. ran is what lets the feed tell that emptiness from a morning
// the rep finished: a found run counts as ran even with zero unanswered
// entries.
//
// Answered entries are dropped here rather than in the feed, because what the
// states mean belongs to the brief. The engine already resolves an expired
// snooze on this read, so an item whose set-aside has run out comes back
// actionable without anything here knowing that rule either.
//
// So are entries whose deal this reader cannot resolve. A run is the night's
// record and keeps every deal it ranked, but the deal can be archived or leave
// the reader's scope between the night and the morning — and the reads that
// would furnish the row (the figures pass and the label resolver) both exclude
// an archived deal, correctly. Left in, the entry reaches the rep as a row with
// no amount, no close date, no reason and no name, still offering act, set
// aside and dismiss over a deal that has been deleted, and counting toward the
// day's total. A row that can name nothing is not a suggestion.
func (a attentionBriefing) Queue(ctx context.Context) ([]attention.BriefEntry, bool, error) {
	run, err := a.engine.LatestRun(ctx, a.now())
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
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
		})
		named = append(named, item.DealID)
	}
	resolvable, err := a.figures.Figures(ctx, named)
	if err != nil {
		return nil, false, err
	}
	entries := make([]attention.BriefEntry, 0, len(unanswered))
	for _, entry := range unanswered {
		if _, ok := resolvable[entry.DealID]; ok {
			entries = append(entries, entry)
		}
	}
	return entries, true, nil
}
