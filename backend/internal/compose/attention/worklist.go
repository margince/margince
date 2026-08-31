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
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// worklistPage is how many ranked items one read carries by default.
const worklistPage = 25

// worklistMaxPage is the ceiling the contract publishes. A larger ask is
// clamped rather than refused: the number is a request for how much to draw,
// and answering the most that can be drawn is more useful than an error.
const worklistMaxPage = 100

// Worklist answers the ranked day.
//
// The filter narrows what is CARRIED, never what is read: a source is read,
// classified and then dropped, so the summary's figures describe the same day
// whichever filter is applied.
func (s *Service) Worklist(ctx context.Context, scope, filter string, limit int) (crmcontracts.Worklist, error) {
	// Resolved BEFORE the day is read: a reader asking for a scope they do not
	// hold gets a refusal rather than a page assembled and then narrowed, and
	// the read they were never entitled to make is not made.
	resolved, err := resolveScope(ctx, scope)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	// The producers that CAN be narrowed are narrowed in their own queries,
	// not filtered afterwards: each store bounds what it returns, so a page
	// full of colleagues' rows would hide the reader's own work behind a cut
	// that had already happened.
	reader := s
	if mineOnly(resolved) {
		reader = s.forReader()
	}
	day, err := reader.Assemble(ctx)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	out := s.worklistFrom(ctx, day, resolved, filter, limit)
	out.Scope = crmcontracts.WorklistScope(resolved)
	out.ScopeOptions = scopeOptions(scopeOptionsFor(ctx))
	return out, nil
}

// keepReadersOwn drops the rows belonging to somebody else.
//
// It runs over the CANDIDATES, before the page is cut, so a reader asking for
// twenty-five of their own rows gets twenty-five where they exist. Narrowing
// after the cut returned a short page while the reader's own work sat just
// past it.
//
// It fails CLOSED. A call with no human behind it has no "own work" to answer
// for, and a queue that handed such a caller every row it had read would be
// widening a scope named `mine` — the opposite of what it says. An agent that
// needs the day reads it under a scope it can actually hold.
//
// The narrowing itself is a display cut, not the security boundary: the lanes
// are already row-scoped, so this changes what a wide-scoped reader is SHOWN
// by default rather than what any store was asked. Pushing the filter into
// each producer is the better shape and needs each to take an owner.
func keepReadersOwn(ctx context.Context, rows []ranked) []ranked {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil
	}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if ownedByReader(row.item, actor) {
			kept = append(kept, row)
		}
	}
	return kept
}

// scopeOptions puts the resolver's answer on the wire.
func scopeOptions(options []string) []crmcontracts.WorklistScopeOptions {
	out := make([]crmcontracts.WorklistScopeOptions, 0, len(options))
	for _, option := range options {
		out = append(out, crmcontracts.WorklistScopeOptions(option))
	}
	return out
}

// worklistFrom projects an already-assembled day, so a test can drive the
// ranking, the paging and the summary without standing up every lane's reader.
func (s *Service) worklistFrom(
	ctx context.Context, day crmcontracts.Attention, scope, filter string, limit int,
) crmcontracts.Worklist {
	if limit <= 0 {
		limit = worklistPage
	}
	if limit > worklistMaxPage {
		limit = worklistMaxPage
	}
	rows := classifyDay(day, day.AsOf)
	if mineOnly(scope) {
		rows = keepReadersOwn(ctx, rows)
	}
	if filter != "" && filter != string(crmcontracts.GetWorklistParamsFilterAll) {
		rows = keepCategory(rows, crmcontracts.WorklistItemCategory(filter))
	}
	// Cut to the page BEFORE explaining and counting. Ranking the whole set and
	// then slicing left the last returned row comparing itself against a row the
	// caller never received, and the summary describing a queue longer than the
	// one on screen.
	ordered := rankAll(page(rows, limit))
	out := crmcontracts.Worklist{
		AsOf:               day.AsOf,
		Queue:              ordered,
		Summary:            summarize(ordered),
		SourcesUnavailable: unavailable(day),
	}
	if filter != "" {
		narrowed := crmcontracts.WorklistFilter(filter)
		out.Filter = &narrowed
	}
	return out
}

// page cuts the candidates to what one read carries.
//
// It runs BEFORE the ranking is drawn, so the comparison each row publishes is
// against a row the caller actually received. The cut itself is by score, so
// this sorts first and then slices — taking the best `limit`, never the first
// `limit` the producers happened to return.
func page(rows []ranked, limit int) []ranked {
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
	if len(rows) > limit {
		return rows[:limit]
	}
	return rows
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
		case item.Level >= levelRoutine:
			summary.LowerPriority++
		}
		if item.Overdue != nil && *item.Overdue {
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
		//
		// This suppression is WIDER than it should be, and the difference is
		// worth stating rather than hiding: the DSR read also refuses a reader
		// who has the admin role but lost `person:read`, and that refusal is
		// real news this list swallows. Telling the two apart needs a reason on
		// the refusal, which the lane contract does not carry — issue filed.
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
