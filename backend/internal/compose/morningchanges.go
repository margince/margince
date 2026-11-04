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
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// morningChangeSource marks a changed deal among the items a morning shows,
// so the send holds the deal and an Undo before the send re-reads the mail.
const morningChangeSource = "margince_change"

// morningChangeLines is how many changed deals the email names; the rest are
// counted and wait on Home.
const morningChangeLines = 5

// morningChangeWindow is "last night": changes older than a day were already
// in an earlier morning.
const morningChangeWindow = 24 * time.Hour

// appendMorningChanges adds the reader's deals Margince changed overnight and
// has not had accepted or undone. A deal the send basis excluded is left out.
func (s *morningSelection) appendMorningChanges(ctx context.Context, store *deals.Store, now time.Time, base string, words mailcopy.Copy) error {
	since := now.Add(-morningChangeWindow)
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
	if len(bundles) == 0 {
		return nil
	}
	s.body.Heading(fmt.Sprintf(words.MorningChanges, len(bundles)+summary.Deals-len(summary.Bundles)))
	for _, bundle := range bundles[:min(morningChangeLines, len(bundles))] {
		s.body.Item(emailview.Item{
			Title: mailcopy.OneLine(bundle.DealName), Verb: words.MailReview, OneLine: true,
			Lines: []emailview.Line{{Text: morningChangeKinds(bundle.Kinds, words)}},
		})
		s.body.ItemAction(words.MailReview, mailcopy.Destination(base, mailcopy.RecordFragment("deal", bundle.DealID.String())))
		s.included = append(s.included, morningItem{source: morningChangeSource, id: bundle.DealID.String()})
		s.shown++
	}
	if more := len(bundles) - morningChangeLines; more > 0 {
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
	}
	var out []string
	for _, kind := range []string{deals.CloseLostCorrection, deals.VerdictCloseDateCorrection, deals.VerdictAmountCorrection, deals.VerdictProbabilityCorrection} {
		for _, have := range kinds {
			if have == kind {
				out = append(out, named[kind])
			}
		}
	}
	return strings.Join(out, " · ")
}
