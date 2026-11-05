// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Team exceptions reuse Worklist classification for revenue risk, unassigned work
// and repeated failures. Waiting age carries no response-policy deadline.

import (
	"context"
	"sort"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// exceptionsBound is how many conditions one page carries.
//
// A page a lead cannot finish is a page they stop opening. Twenty-five is the
// queue's own page size, which is the number this product has already decided
// a reader reads in one sitting — and `truncated` says when it was reached, so
// a bounded page is never read as a clear team.
const exceptionsBound = 25

// TeamExceptions is what a lead can act on across their team.
func (s *Service) TeamExceptions(ctx context.Context) (crmcontracts.TeamExceptions, error) {
	// ONE snapshot, and this page needs it most: it reads the roster, the whole
	// assembled day and the money on top, which measured as 54 transactions per
	// request against the worklist's 33 (margince#4912).
	// Admission BEFORE the snapshot, for the reason worklist.go gives.
	if err := requireLeadTier(ctx); err != nil {
		return crmcontracts.TeamExceptions{}, err
	}
	var out crmcontracts.TeamExceptions
	err := s.inSnapshot(ctx, ReadTeamExceptions, func(ctx context.Context) error {
		var err error
		out, err = s.teamExceptionsIn(ctx)
		return err
	})
	return out, err
}

// teamExceptionsIn is TeamExceptions' body, inside the snapshot it opened.
func (s *Service) teamExceptionsIn(ctx context.Context) (crmcontracts.TeamExceptions, error) {
	if s.teammates == nil {
		// A REFUSAL, the same answer TeamBoard gives: a page assembled without
		// the membership reader has no team to answer for, and reporting that
		// as a clear one would tell a lead their team is fine when nobody
		// looked.
		return crmcontracts.TeamExceptions{}, apperrors.ErrPermissionDenied
	}
	roster, rosterCut, err := s.teammates.LiveTeammatesOfCaller(ctx)
	if err != nil {
		return crmcontracts.TeamExceptions{}, err
	}
	former, err := s.formerTeammates(ctx)
	if err != nil {
		return crmcontracts.TeamExceptions{}, err
	}
	day, err := s.Assemble(ctx)
	if err != nil {
		return crmcontracts.TeamExceptions{}, err
	}
	if err := s.nameTheMoney(ctx, &day); err != nil {
		return crmcontracts.TeamExceptions{}, err
	}
	money, err := s.priceTheDay(ctx, day)
	if err != nil {
		return crmcontracts.TeamExceptions{}, err
	}
	admitted, refused := admitBySource(ctx, classifyDay(day, day.AsOf, money))
	found := exceptionsFor(rowsForRoster(admitted, roster, former...), day.AsOf)
	out := crmcontracts.TeamExceptions{
		AsOf:       day.AsOf,
		Exceptions: found,
		Truncated:  rosterCut || len(refused) > 0 || len(found) > exceptionsBound,
	}
	if len(found) > exceptionsBound {
		out.Exceptions = found[:exceptionsBound]
	}
	nameExceptionOwners(out.Exceptions, rosterNames(roster))
	return out, nil
}

// nameExceptionOwners names each row's owner from the roster this page already
// read, as the queue names its own. Unnamed, a client draws a real colleague as
// "Hidden", which is the word it keeps for an owner this reader may not resolve.
func nameExceptionOwners(found []crmcontracts.TeamException, names map[ids.UUID]string) {
	for i := range found {
		nameAnOwner(&found[i].Owner, names)
	}
}

// exceptionsIn reads the conditions out of an assembled day.
//
// FROM THE DAY the queue already assembles, not a second set of queries. Every
// row here passed the same gated lanes the rep's own queue reads, so a lead
// sees exceptions over work they may already see and the two surfaces answer
// one question rather than two. A parallel read would be a second answer, and
// the two would drift the first time either changed.
func exceptionsIn(rows []ranked, asOf time.Time) []crmcontracts.TeamException {
	out := []crmcontracts.TeamException{}
	for _, row := range rows {
		if at, found := exceptionOf(row, asOf); found {
			out = append(out, at)
		}
	}
	return out
}

// exceptionsFor is the page's own answer: what is going wrong, worst first.
//
// The raise and the ordering are ONE step because a caller cannot want the
// first without the second — an unordered list of exceptions is a list a lead
// reads top to bottom looking for the one that matters. Splitting them let a
// test call the comparator itself and pass while the page returned whatever
// order the lanes happened to produce, which is what this shape prevents.
func exceptionsFor(rows []ranked, asOf time.Time) []crmcontracts.TeamException {
	found := exceptionsIn(rows, asOf)
	sortExceptions(found)
	return found
}

// Kind orders unlike risks; age orders rows only within the same kind.
func sortExceptions(found []crmcontracts.TeamException) {
	sort.SliceStable(found, func(i, j int) bool {
		a, b := exceptionRank(found[i].Kind), exceptionRank(found[j].Kind)
		if a != b {
			return a < b
		}
		return found[i].Since.Before(found[j].Since)
	})
}

// Reserved contract values keep a deterministic order alongside active kinds.
func exceptionRank(kind crmcontracts.TeamExceptionKind) int {
	switch kind {
	case crmcontracts.TeamExceptionKindTeamExceptionResponseBreached:
		return 0
	case crmcontracts.TeamExceptionKindTeamExceptionRevenueAtRisk:
		return 1
	case crmcontracts.TeamExceptionKindTeamExceptionUnassigned:
		return 2
	default:
		return 3
	}
}

// exceptionOf says whether one row is a condition a lead can act on, and what
// it was judged against.
func exceptionOf(row ranked, asOf time.Time) (crmcontracts.TeamException, bool) {
	// A condition with no record behind it is not one a lead can act on: every
	// intervention this page offers — talk to the owner, protect the deal, give
	// it an owner — needs something to open. A row with no subject is real work
	// on the rep's own queue and simply not this page's business.
	if row.item.Subject == nil {
		return crmcontracts.TeamException{}, false
	}
	owner := ownerOnTheWire(row, ids.UUID{})
	switch {
	// Revenue the day already judged material — the pipeline's own median,
	// which is what makes "material" track the business rather than a number
	// somebody typed once.
	case row.item.Source == sourceAtRisk && row.item.Level <= levelMaterialRisk:
		return exception(row, owner, crmcontracts.TeamExceptionKindTeamExceptionRevenueAtRisk,
			thresholdMaterial, asOf), true
	// Work nobody has taken. Stated by the producer rather than inferred: an
	// unstated owner is a lane that never answered, which is not the same as
	// nobody holding the row.
	case owner != nil && owner.Kind == crmcontracts.WorklistOwnerKindWorklistOwnerUnassigned:
		return exception(row, owner, crmcontracts.TeamExceptionKindTeamExceptionUnassigned,
			thresholdUnowned, asOf), true
	// One broken thing reported many times. The fold already decided these are
	// one condition rather than a pile, so the count is the evidence.
	case row.item.Batch != nil && row.item.Batch.Key == keySystemIncident:
		return exception(row, owner, crmcontracts.TeamExceptionKindTeamExceptionRepeatedFailure,
			thresholdRepeated, asOf), true
	}
	return crmcontracts.TeamException{}, false
}

// The bases the three non-policy kinds are judged against, in words a lead can
// check. Spelled as constants because the contract promises every row names
// one, and a literal at each site is how one of them quietly stops matching.
const (
	thresholdMaterial = "at or above the pipeline's median open deal"
	thresholdUnowned  = "no owner stated by the lane that raised it"
	thresholdRepeated = "folded into one incident by the queue's own grouping"
)

// exception builds the row, carrying what the queue already knows about it.
func exception(
	row ranked, owner *crmcontracts.WorklistOwner,
	kind crmcontracts.TeamExceptionKind, threshold string, asOf time.Time,
) crmcontracts.TeamException {
	at := crmcontracts.TeamException{
		Kind:        kind,
		Subject:     *row.item.Subject,
		Since:       exceptionSince(row, asOf),
		Consequence: string(row.item.Consequence),
		Threshold:   threshold,
	}
	if owner != nil {
		at.Owner = *owner
	}
	// The producer's own line, where it has one. Absent rather than invented:
	// a row that cannot say what it saw says nothing, and the lead reads the
	// subject and the clock instead.
	if row.item.Detail != nil && *row.item.Detail != "" {
		at.Evidence = row.item.Detail
	}
	return at
}

// exceptionSince is when the condition started.
//
// The row's own occurrence where it has one, falling back to the read instant.
// Never zero: a lead reading "since" as the moment a clock started cannot be
// handed the zero time, which renders as a date in year one and reads as a
// fault in the product rather than a missing fact.
func exceptionSince(row ranked, asOf time.Time) time.Time {
	if !row.occurredAt.IsZero() {
		return row.occurredAt
	}
	return asOf
}
