// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What is going wrong on a lead's team.
//
// The team board answers "who is carrying what" and routes a lead to a person.
// This answers "what is going wrong", which is the question they open the page
// for and which counts structurally cannot reach: three numbers per teammate
// cannot say that one customer has waited past the target while another rep's
// queue is merely long.
//
// EVERY ROW NAMES ITS BASIS. `threshold` carries the policy state that decided
// the row — the lead-response policy's own vocabulary, the same one the rep's
// queue reads — rather than a number chosen for this reading. A lead disputing
// a row can then see the rule instead of the verdict, and the manager and the
// rep hold one rule rather than two that agree today and drift tomorrow.
//
// CAPACITY IS DELIBERATELY ABSENT. "This rep is overloaded" needs a configured
// capacity to be a fact rather than an opinion, and this installation has none.
// The board's counts stay a reading, and no exception here claims otherwise.

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
// a person reads in one sitting — and `truncated` says when it was reached, so
// a bounded page is never read as a clear team.
const exceptionsBound = 25

// TeamExceptions is what a lead can act on across their team.
func (s *Service) TeamExceptions(ctx context.Context) (crmcontracts.TeamExceptions, error) {
	if err := requireLeadTier(ctx); err != nil {
		return crmcontracts.TeamExceptions{}, err
	}
	if s.teammates == nil {
		// A REFUSAL, the same answer TeamBoard gives: a page assembled without
		// the membership reader has no team to answer for, and reporting that
		// as a clear one would tell a lead their team is fine when nobody
		// looked.
		return crmcontracts.TeamExceptions{}, apperrors.ErrPermissionDenied
	}
	day, err := s.Assemble(ctx)
	if err != nil {
		return crmcontracts.TeamExceptions{}, err
	}
	rows := classifyDay(day, day.AsOf, dayMoney{})
	found := exceptionsIn(rows, day.AsOf)
	sortExceptions(found)
	out := crmcontracts.TeamExceptions{
		AsOf:       day.AsOf,
		Exceptions: found,
		Truncated:  len(found) > exceptionsBound,
	}
	if out.Truncated {
		out.Exceptions = found[:exceptionsBound]
	}
	return out, nil
}

// exceptionsIn reads the conditions out of an assembled day.
//
// FROM THE DAY the queue already assembles, not a second set of queries. Every
// row here passed the same gated lanes the rep's own queue reads, so a lead
// sees exceptions over work they may already see and the two surfaces answer
// one question rather than two. A parallel read would be a second answer, and
// the two would drift the first time either changed.
//
// Held by: TestABreachedReplyIsJudgedByThePolicysOwnState
// (teamexceptions_test.go), which fails if this page decides a breach from
// anything but the lead lane's own verdict.
func exceptionsIn(rows []ranked, asOf time.Time) []crmcontracts.TeamException {
	out := []crmcontracts.TeamException{}
	for _, row := range rows {
		if at, found := exceptionOf(row, asOf); found {
			out = append(out, at)
		}
	}
	return out
}

// sortExceptions puts the worst first.
//
// By KIND rather than by age, because the four are not comparable on one clock:
// a breached response and an unowned deal are different kinds of wrong, and
// ordering them by how long they have been wrong would put a week-old piece of
// hygiene above a customer who has waited since this morning. Within a kind the
// oldest leads, which is the only comparison that means anything there.
func sortExceptions(found []crmcontracts.TeamException) {
	sort.SliceStable(found, func(i, j int) bool {
		a, b := exceptionRank(found[i].Kind), exceptionRank(found[j].Kind)
		if a != b {
			return a < b
		}
		return found[i].Since.Before(found[j].Since)
	})
}

// exceptionRank is the order a lead reads the four kinds in.
func exceptionRank(kind crmcontracts.TeamExceptionKind) int {
	switch kind {
	case crmcontracts.TeamExceptionResponseBreached:
		return 0
	case crmcontracts.TeamExceptionRevenueAtRisk:
		return 1
	case crmcontracts.TeamExceptionUnassigned:
		return 2
	default:
		return 3
	}
}

// exceptionOf says whether one row is a condition a lead can act on, and what
// it was judged against.
//
// FOUR KINDS, and each is a thing a lead can DO something about: talk to the
// person, protect the revenue, give the work an owner, or fix what keeps
// failing. A row that is merely urgent for the rep is not an exception — the
// rep's own queue already ranks it, and repeating it here would make this page
// a second copy of theirs with a different heading.
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
	// A first reply the policy says is already late. The THRESHOLD is that
	// policy's own state, so the manager and the rep read one rule.
	case row.item.Source == sourceLeadResponse && breachedReply(row):
		return exception(row, owner, crmcontracts.TeamExceptionResponseBreached,
			string(crmcontracts.LeadSlaStateBreached), asOf), true
	// Revenue the day already judged material — the pipeline's own median,
	// which is what makes "material" track the business rather than a number
	// somebody typed once.
	case row.item.Source == sourceAtRisk && row.item.Level <= levelMaterialRisk:
		return exception(row, owner, crmcontracts.TeamExceptionRevenueAtRisk,
			thresholdMaterial, asOf), true
	// Work nobody has taken. Stated by the producer rather than inferred: an
	// unstated owner is a lane that never answered, which is not the same as
	// nobody holding the row.
	case owner != nil && owner.Kind == crmcontracts.WorklistOwnerUnassigned:
		return exception(row, owner, crmcontracts.TeamExceptionUnassigned,
			thresholdUnowned, asOf), true
	// One broken thing reported many times. The fold already decided these are
	// one condition rather than a pile, so the count is the evidence.
	case row.item.Batch != nil && row.item.Batch.Key == keySystemIncident:
		return exception(row, owner, crmcontracts.TeamExceptionRepeatedFailure,
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

// breachedReply reads the lead lane's own verdict off the row.
//
// The reason the classifier already wrote, not a second reading of the clock:
// leadStanding decides breach from the policy state, and re-deciding it here
// from the deadline would be a second rule that agrees today.
func breachedReply(row ranked) bool {
	for _, because := range row.item.Because {
		if because.Kind == "response_overdue" {
			return true
		}
	}
	return false
}
