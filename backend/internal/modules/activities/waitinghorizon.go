// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// How far back a wait can reach and still be work, measured from what this
// installation actually does rather than compiled in for everybody.
//
// One number judged every installation: an agency answering within the hour and
// an enterprise seller working sixty-day cycles got the same ninety days. For
// the agency that is a queue holding conversations everyone has forgotten; for
// the enterprise it is a queue dropping live business on a date nobody chose.
//
// A SETTING was the obvious answer and it is the wrong one. Response
// expectations plausibly differ per pipeline, per deal size, per customer tier,
// so a single workspace number papers over that with an authoritative face —
// and a number nobody revisits is the failure material_threshold_minor already
// solved the same way: derive the bar from the data rather than ask for it.
//
// A derived horizon also makes no promise. Nothing escalates when work crosses
// it, because nothing was committed to — which is why this needs no new lane
// and no question about who is told.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// waitingHorizonSpread is what the derivation measures: how long this
// installation took to answer, and over how many answers.
type waitingHorizonSpread struct {
	// Slow is the high percentile of first-response times — the point past
	// which this installation essentially does not answer. The MEDIAN would be
	// the wrong input: half of all answers arrive after it, so a horizon set
	// there would drop conversations that routinely get replies.
	Slow time.Duration
	// Answered is how many first responses that percentile was taken over.
	Answered int
}

const (
	// waitingHorizonMinSample is how many answered threads the derivation needs
	// before it will speak.
	//
	// A fresh installation has no history at all, and a thin one produces a
	// wild number from a handful of conversations that happen to be slow. Below
	// this the compiled ninety stands — stated here rather than left to emerge
	// from a percentile over three rows.
	waitingHorizonMinSample = 30

	// waitingHorizonFloorDays and waitingHorizonCeilingDays bound what the
	// measurement may produce.
	//
	// The ceiling is the queue's other failure mode: a derivation with none can
	// answer "keep everything forever", which is not a queue. The floor is the
	// mirror — an installation that answers everything within a day would
	// otherwise derive a horizon that drops a customer who waited a week, and a
	// week's silence is not history in any business.
	waitingHorizonFloorDays   = 14
	waitingHorizonCeilingDays = 365

	// waitingHorizonSlack is how much longer than its slow answers an
	// installation keeps waiting.
	//
	// The horizon is not "how long we take" — a wait exactly as old as our
	// slowest answer is one still plausibly about to be answered. It is how
	// long past that a thread stays worth looking at, and three times is the
	// coarse reading this deliberately keeps coarse: the bands that separate an
	// urgent wait from a stale one are the caller's, and a horizon precise to
	// the hour would imply a judgement this measurement cannot make.
	waitingHorizonSlack = 3
)

// derivedWaitingHorizonDays is the horizon this installation's own behaviour
// implies, and waitingHorizonDays when it has not answered enough to say.
//
// Pure, and separate from the query that feeds it, because the arithmetic is
// the part worth being able to read and test: what it measures, what it does
// with too little data, and what it refuses to produce.
func derivedWaitingHorizonDays(spread waitingHorizonSpread) int {
	if spread.Answered < waitingHorizonMinSample || spread.Slow <= 0 {
		return waitingHorizonDays
	}
	days := int(spread.Slow.Hours()/24) * waitingHorizonSlack
	return min(max(days, waitingHorizonFloorDays), waitingHorizonCeilingDays)
}

const (
	// medianPercentile is what the metrics window asks firstResponseSQL for:
	// the middle answer, which is what "how fast do we answer" means to a
	// person reading a number.
	medianPercentile = "0.5"
	// slowPercentile is what the horizon asks it for. Not the median — half of
	// all answers arrive after that, so a horizon set there would drop
	// conversations this installation routinely replies to. The ninety-fifth
	// is the point past which it essentially does not answer, and the last five
	// per cent are left out because one abandoned thread answered a year late
	// would otherwise set the horizon for everybody.
	slowPercentile = "0.95"

	// waitingHorizonWindowDays is how far back the measurement looks.
	//
	// A year, so the derivation follows a business that has changed rather than
	// one it used to be, and so a seasonal quarter cannot speak for the whole
	// installation. It is deliberately longer than any horizon this can
	// produce: the measurement's window and the horizon it derives are
	// different things, and tying them together would make the horizon depend
	// on its own previous value.
	waitingHorizonWindowDays = 365
)

// waitingHorizonFor measures this installation's own response spread and
// derives the horizon from it, inside the caller's transaction.
//
// UNGATED, and that is the one thing about this read worth arguing. Every other
// figure in this package is taken under the caller's own visibility, because it
// is about their work. The horizon is not: it decides which rows the queue
// contains, so two colleagues reading one shared thread have to be judged by
// one number. Derived per reader it would make the queue's contents depend on
// who is looking, twice over and invisibly.
//
// What that costs is a duration and a count over the installation, and no
// content: nothing here names a conversation, a person or a record.
func (s *Store) waitingHorizonFor(ctx context.Context, tx pgx.Tx, asOf time.Time) (int, error) {
	args := []any{asOf.AddDate(0, 0, -waitingHorizonWindowDays), asOf}
	arg := func(v any) int { args = append(args, v); return len(args) }
	ownDomains, err := s.ownDomainList(ctx, tx)
	if err != nil {
		return 0, err
	}
	var spread waitingHorizonSpread
	var slowMinutes int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(firstResponseSQL,
		scopeUnbounded,
		liveRecord(openDealPredicate, "d"),
		liveRecord(workingLeadPredicate, "ld"),
		ownDomainSenderSQL("inbound", arg(ownDomains)),
		slowPercentile),
		args...).Scan(&spread.Answered, &slowMinutes); err != nil {
		return 0, fmt.Errorf("activities: measuring the response spread the horizon follows: %w", err)
	}
	spread.Slow = time.Duration(slowMinutes) * time.Minute
	return derivedWaitingHorizonDays(spread), nil
}

// horizonOrDefault reads an unmeasured horizon as the compiled one.
//
// A clause builder is reached from paths that hold no transaction to measure
// through, and the honest answer for those is today's behaviour rather than a
// horizon of zero days — which would call every wait history and empty the
// queue.
func horizonOrDefault(days int) int {
	if days <= 0 {
		return waitingHorizonDays
	}
	return days
}
