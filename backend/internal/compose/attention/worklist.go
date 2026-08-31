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
	"errors"
	"log/slog"
	"sort"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// worklistPage is how many ranked items one read carries by default.
const worklistPage = 25

// waitingLead is how many unanswered customers lead the page.
//
// Not a cap on the source: the rest are still ranked and still reachable. It is
// a cap on how much of ONE kind a reader meets before they see the others, and
// the number is the answer to "how many can somebody act on this morning"
// rather than to "how many are there".
const waitingLead = 8

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
	// Deeper than the lane feed reads: a batch row counts a pile, and a count
	// taken from a page of ten would report ten over a hundred and fifty.
	reader := s.countingDecisions()
	if mineOnly(resolved) {
		reader = reader.forReader()
	}
	day, err := reader.Assemble(ctx)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	// Read beside the assembled day rather than inside it: /attention has its
	// own fourteen-lane promise and this source is not one of its lanes. A
	// refused read is named, never folded into an empty answer.
	waiting, waitingErr := reader.waitingCustomers(ctx, day.AsOf)
	out := s.worklistFrom(ctx, day, resolved, filter, limit, waiting)
	if waitingErr != nil {
		out.SourcesUnavailable = append(out.SourcesUnavailable, *waitingErr)
	}
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
// waitingCustomers reads who is waiting, or names why it could not.
//
// A refusal is reported as a withheld source; any other failure as a failed
// one. Neither takes the rest of the day down with it — a page that answered
// nothing because one source stumbled is less useful than a page that says
// which part it could not read.
func (s *Service) waitingCustomers(
	ctx context.Context, asOf time.Time,
) ([]WaitingCustomer, *crmcontracts.WorklistSourceUnavailable) {
	if s.waiting == nil {
		return nil, nil
	}
	rows, err := s.waiting.Unanswered(ctx, asOf)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return nil, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceWaiting, Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
		}
	case err != nil:
		// Named on the page AND recorded here. A source reported as failed with
		// nothing in the log leaves an operator with a warning they cannot act
		// on — which is how this one went a whole verification round without
		// anybody being able to say what broke.
		slog.ErrorContext(ctx, "the who-is-waiting read failed", "error", err)
		return nil, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceWaiting, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
		}
	default:
		return rows, nil
	}
}

func (s *Service) worklistFrom(
	ctx context.Context, day crmcontracts.Attention, scope, filter string, limit int,
	waiting []WaitingCustomer,
) crmcontracts.Worklist {
	if limit <= 0 {
		limit = worklistPage
	}
	if limit > worklistMaxPage {
		limit = worklistMaxPage
	}
	rows := classifyDay(day, day.AsOf)
	// A waiting message has no owner of its own, so under `mine` its ownership
	// is the ownership of the record it is filed under: a thread about a
	// colleague's deal is that colleague's to answer. Judged against the deals
	// this reader owns in THIS day, which is the set `mine` already narrowed.
	owned := ownedDealsIn(ctx, day, scope)
	// Longest wait first, so the few that LEAD are the ones most likely to have
	// been forgotten rather than whichever the database returned first.
	mine := make([]WaitingCustomer, 0, len(waiting))
	for _, customer := range waiting {
		if mineOnly(scope) && !waitingIsMine(customer, owned) {
			continue
		}
		mine = append(mine, customer)
	}
	sort.SliceStable(mine, func(i, j int) bool { return mine[i].Since.Before(mine[j].Since) })
	for i, customer := range mine {
		row := classifyWaiting(customer, day.AsOf)
		// Past the lead, a wait sorts BELOW the other kinds without ceasing to
		// be one: its level still says a customer is waiting, because that is
		// what it is and the summary counts on it. What changes is only where
		// it sits, through the ordering's own last tiebreak.
		//
		// Rewriting the level instead would have told the reader the ninth
		// waiting customer was agreed work, while the row went on saying a
		// buyer wrote last — the page contradicting itself.
		if i >= waitingLead {
			row.crowded = true
		}
		rows = append(rows, row)
	}
	// One unanswered message is one row: the deal it belongs to does not also
	// appear as drifting.
	rows = dropDealsAlreadyWaiting(rows)

	if mineOnly(scope) {
		rows = keepReadersOwn(ctx, rows)
	}
	narrowed := filter != "" && filter != string(crmcontracts.GetWorklistParamsFilterAll)
	if narrowed {
		rows = keepCategory(rows, crmcontracts.WorklistItemCategory(filter))
	}
	// A pile of alike routine decisions is one row, not a hundred — but ONLY on
	// the unnarrowed page. A reader who asked for decisions asked to see them,
	// and answering that with the same group they were trying to open is a door
	// that leads back to itself. Narrowing IS opening the group.
	// Held before the fold, because folding REPLACES rows: counting after it
	// would report a folded source as having considered nothing, which is the
	// opposite of what happened to it.
	considered := rows
	if !narrowed {
		// The decision read stops at its own scan bound, so a group that filled
		// it reports a floor rather than a total.
		rows = foldRoutineDecisionsBounded(rows, len(day.NeedsYou) >= batchScanDepth)
	}
	// Cut to the page BEFORE explaining and counting. Ranking the whole set and
	// then slicing left the last returned row comparing itself against a row the
	// caller never received, and the summary describing a queue longer than the
	// one on screen.
	shown := page(rows, limit)
	ordered := rankAll(shown)
	out := crmcontracts.Worklist{
		AsOf:               day.AsOf,
		Queue:              ordered,
		Summary:            summarize(ordered),
		SourcesUnavailable: unavailable(day),
		// `considered` is every candidate this read weighed, `shown` what
		// survived folding and the cut. Both are already in hand, so no figure
		// here costs a query that could disagree with the page it describes.
		Reach: reachOf(considered, shown, boundedSources(day)),
	}
	if filter != "" {
		narrowed := crmcontracts.WorklistFilter(filter)
		out.Filter = &narrowed
	}
	return out
}

// ownedDealsIn is the set of deals on this day that the reader owns.
//
// Read off the assembled day rather than queried again: the at-risk lane has
// already returned the deals under the reader's own scope, so this is the
// answer that surface already has rather than a second opinion about it.
// boundedSources names the lanes that came back exactly at their own work
// bound, and therefore may have had more behind them.
//
// A lane read to its limit cannot tell a reader how many it did not see: the
// bound is a limit on work, not a count. So the source is marked as having more
// rather than reporting a total it does not know, and every figure beside it
// stays a count of what this read actually weighed.
func boundedSources(day crmcontracts.Attention) map[crmcontracts.WorklistItemSource]bool {
	bounded := map[crmcontracts.WorklistItemSource]bool{}
	atCap := func(source crmcontracts.WorklistItemSource, lane *[]crmcontracts.AttentionItem, cap int) {
		if lane != nil && len(*lane) >= cap {
			bounded[source] = true
		}
	}
	atCap("failed_approval", day.DidNotRun, doneCap)
	atCap("dsr", day.Dsr, doneCap)
	atCap("ai_work_health", day.AiWorkHealth, doneCap)
	atCap("notice", day.Notices, doneCap)
	atCap("automation_run", day.AutomationHealth, doneCap)
	atCap("bounce", day.Bounces, doneCap)
	// The decision lane is read deeper than the rest, because a batch row
	// counts a pile and a count taken from a page of ten would report ten over
	// a hundred and fifty.
	if len(day.NeedsYou) >= batchScanDepth {
		bounded["approval"] = true
		bounded["dedupe_candidate"] = true
	}
	return bounded
}

func ownedDealsIn(ctx context.Context, day crmcontracts.Attention, scope string) map[ids.UUID]bool {
	owned := map[ids.UUID]bool{}
	if !mineOnly(scope) || day.AtRisk == nil {
		return owned
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return owned
	}
	for _, item := range *day.AtRisk {
		if item.Deal == nil || item.Deal.OwnerId == nil || item.Subject == nil {
			continue
		}
		if ids.UUID(*item.Deal.OwnerId) == actor.UserID {
			owned[ids.UUID(item.Subject.Id)] = true
		}
	}
	return owned
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
			Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
		})
	}
	return out
}

// laneDSR is the one lane whose withholding is a permanent role fact rather
// than news about this reader's day.
const laneDSR = crmcontracts.AttentionLanesOmitted("dsr")
