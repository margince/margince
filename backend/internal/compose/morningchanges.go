// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deals Margince changed overnight, as a block of the morning email.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// morningChangeSource marks a changed deal among the items a morning shows.
// The send holds the deal, so an Undo before the send re-reads the mail.
const morningChangeSource = "margince_change"

// morningChangesWindow marks where the counted changes start; its id is the
// instant, and the send holds every change since it.
const morningChangesWindow = "margince_changes_since"

// morningChangeLines is how many changed deals the email names; the rest are
// counted and wait on Home.
const morningChangeLines = 5

// morningChangeWindow is "last night" for a reader who has had no morning
// mail yet.
const morningChangeWindow = 24 * time.Hour

// morningCorrectionJobs are the jobs that write the changes the morning
// counts. The close-date sweep runs the verdict and the silent close; the deal
// scout opens and reopens deals.
var morningCorrectionJobs = []string{CloseDateSweepArgs{}.Kind(), DealScoutArgs{}.Kind()}

// morningChangeOverlap is the longest a correction writer's transaction can
// stay open: the longest timeout of those jobs.
func morningChangeOverlap() time.Duration {
	var longest time.Duration
	for _, kind := range morningCorrectionJobs {
		if spec, ok := jobs.SpecFor(kind); ok {
			longest = max(longest, spec.Timeout.Duration())
		}
	}
	return longest
}

// morningChangesSince is where "last night" starts: the reader's previous
// morning mail, less the overlap. A change stamped before that mail can commit
// after it, and the overlap keeps it in this morning. Such a change may show
// in two mornings; one accepted or undone shows in neither.
func morningChangesSince(previous *time.Time, now time.Time) time.Time {
	if previous == nil {
		return now.Add(-morningChangeWindow)
	}
	return previous.Add(-morningChangeOverlap())
}

// appendMorningChanges adds the reader's deals Margince changed since the
// previous morning and has not had accepted or undone. A deal the send basis
// excluded is left out.
func (s *morningSelection) appendMorningChanges(ctx context.Context, store *deals.Store, since time.Time, base string, words mailcopy.Copy) error {
	summary, err := store.PendingChangesOwnedBy(ctx, &since)
	if err != nil {
		return err
	}
	var bundles []deals.ChangeBundle
	for _, bundle := range summary.Bundles {
		if !s.excluded[bundle.DealID.UUID] {
			bundles = append(bundles, bundle)
		}
	}
	// The total is SQL's count over every changed deal, less the ones the
	// basis excluded; the 500-bundle cap never shrinks it.
	total := summary.Deals - (len(summary.Bundles) - len(bundles))
	if total <= 0 {
		return nil
	}
	// The whole set behind the count is held, not only the deals listed.
	s.included = append(s.included, morningItem{source: morningChangesWindow, id: since.UTC().Format(time.RFC3339Nano)})
	s.body.Heading(fmt.Sprintf(words.MorningChanges, total))
	for _, bundle := range bundles[:min(morningChangeLines, len(bundles))] {
		s.body.Item(emailview.Item{
			Title: mailcopy.OneLine(bundle.DealName), Verb: words.MailReview, OneLine: true,
			Lines: []emailview.Line{{Text: morningChangeKinds(bundle.Kinds, words)}},
		})
		s.body.ItemAction(words.MailReview, mailcopy.Destination(base, mailcopy.RecordFragment("deal", bundle.DealID.String())))
		s.included = append(s.included, morningItem{source: morningChangeSource, id: bundle.DealID.String()})
		s.shown++
	}
	if more := total - min(morningChangeLines, len(bundles)); more > 0 {
		s.body.Paragraph(fmt.Sprintf(words.MorningChangeMore, more))
	}
	s.body.Action(words.MorningChangesUndo, mailcopy.Destination(base, mailcopy.BriefMorningFragment))
	return nil
}

// morningChangeKinds names what changed on one deal, in a fixed order.
func morningChangeKinds(kinds []string, words mailcopy.Copy) string {
	named := map[string]string{
		deals.CloseLostCorrection:          words.MorningChangeClosed,
		deals.VerdictCloseDateCorrection:   words.MorningChangeRedated,
		deals.VerdictAmountCorrection:      words.MorningChangeRevalued,
		deals.VerdictProbabilityCorrection: words.MorningChangeProbability,
		deals.VerdictStageMoveCorrection:   words.MorningChangeMoved,
		deals.DealOpenCorrection:           words.MorningChangeOpened,
		deals.DealReviveCorrection:         words.MorningChangeReopened,
	}
	var out []string
	for _, kind := range []string{
		deals.DealOpenCorrection, deals.DealReviveCorrection, deals.CloseLostCorrection, deals.VerdictStageMoveCorrection, deals.VerdictCloseDateCorrection,
		deals.VerdictAmountCorrection, deals.VerdictProbabilityCorrection,
	} {
		for _, have := range kinds {
			if have == kind {
				out = append(out, named[kind])
			}
		}
	}
	return strings.Join(out, " · ")
}
