// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The Worklist: the same day the lane feed reads, projected as ONE ranked queue.
//
// It reads through Assemble rather than beside it. Two readers of one day would
// be two answers to "what is waiting on me", and they would drift the first time
// a lane changed — so this is a PROJECTION of the assembled day, and a lane
// added there reaches the queue by being classified here rather than by being
// read again.
//
// What it adds is the part a lane feed cannot: a level, a reason, and a
// consequence. Those are what let a reader compare a duplicate merge with an
// unanswered buyer without reading fourteen panels first.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// worklistPage is how many ranked items one read carries by default.
const worklistPage = 25

// Worklist answers the ranked day.
//
// The filter narrows what is CARRIED, never what is read: a source is read,
// classified and then dropped, so the summary's figures describe the same day
// whichever filter is applied.
func (s *Service) Worklist(ctx context.Context, filter string, limit int) (crmcontracts.Worklist, error) {
	day, err := s.Assemble(ctx)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	if limit <= 0 || limit > 100 {
		limit = worklistPage
	}
	rows := classifyDay(day, day.AsOf)
	if filter != "" && filter != string(crmcontracts.All) {
		rows = keepCategory(rows, crmcontracts.WorklistItemCategory(filter))
	}
	ordered := rankAll(rows)
	out := crmcontracts.Worklist{
		AsOf:               day.AsOf,
		Queue:              page(ordered, limit),
		Summary:            summarize(ordered),
		SourcesUnavailable: unavailable(day),
	}
	if filter != "" {
		narrowed := crmcontracts.WorklistFilter(filter)
		out.Filter = &narrowed
	}
	return out, nil
}

// page cuts the ranked list to what one read carries. The order is already
// decided, so this is a slice and never a second sort.
func page(items []crmcontracts.WorklistItem, limit int) []crmcontracts.WorklistItem {
	if len(items) > limit {
		return items[:limit]
	}
	// Never nil: the contract declares an array, and a null would break a
	// generated client that iterates what the schema promised was a list.
	if items == nil {
		return []crmcontracts.WorklistItem{}
	}
	return items
}

func keepCategory(rows []ranked, want crmcontracts.WorklistItemCategory) []ranked {
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if row.item.Category == want {
			kept = append(kept, row)
		}
	}
	return kept
}

// summarize counts the day in the three figures the header states.
//
// Every figure counts items the queue actually CARRIES, so a number above the
// list and the rows below it cannot disagree — which is the defect the lane
// feed shipped, reporting a twelve-item page as a total.
//
// Held by: TestTheSummaryCountsTheSameItemsTheQueueCarries
// (backend/internal/compose/attention/worklist_test.go).
func summarize(items []crmcontracts.WorklistItem) crmcontracts.WorklistSummary {
	summary := crmcontracts.WorklistSummary{Total: len(items)}
	for _, item := range items {
		switch {
		case item.Level <= levelPromise:
			summary.Urgent++
		case item.Level >= levelBlocking:
			summary.LowerPriority++
		}
		if item.Overdue != nil && *item.Overdue {
			summary.Due++
			continue
		}
		if item.DueAt != nil {
			summary.Due++
		}
	}
	return summary
}

// unavailable turns the assembled day's withheld lanes into the queue's own
// vocabulary.
//
// The lane feed already names what a caller may not read; this widens the same
// promise to say WHY. A day cannot read as clear while something that would
// have filled it never answered, which is the one lie a worklist must not tell.
func unavailable(day crmcontracts.Attention) []crmcontracts.WorklistSourceUnavailable {
	out := []crmcontracts.WorklistSourceUnavailable{}
	if day.LanesOmitted == nil {
		return out
	}
	for _, lane := range *day.LanesOmitted {
		// The DSR lane is withheld BY ROLE for every reader who is not a
		// privacy admin, permanently and by design. Naming it would put "part
		// of your day is hidden" on every rep's page forever, which drowns the
		// warning this list exists to give.
		if lane == laneDSR {
			continue
		}
		out = append(out, crmcontracts.WorklistSourceUnavailable{
			Source: string(lane),
			Reason: crmcontracts.Withheld,
		})
	}
	return out
}

// laneDSR is the one lane whose withholding is a permanent role fact rather
// than news about this reader's day.
const laneDSR = crmcontracts.AttentionLanesOmitted("dsr")
