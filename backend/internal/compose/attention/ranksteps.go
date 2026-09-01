// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The ordering, as ONE sequence of steps.
//
// The queue makes two promises about every pair of rows: which one leads, and
// why. Those were two functions walking the same steps by hand, and the hand
// slipped — `crowded` was the first thing the ordering compared and the one
// thing the explanation never mentioned, so past the eighth waiting customer a
// row reported "nothing decided this" while crowding had decided it.
//
// Now each step is one value that knows both halves: `decides` answers whether
// this step separates the pair and which way, `explain` says so in the
// contract's own words. less() and compare() both walk this slice, so a step
// added to one is a step added to both, and a step that can decide without
// being able to describe itself cannot be written.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// rankStep is one tie-break: how it orders a pair, and how it explains itself.
type rankStep struct {
	// name is for the reader of a failure, not for the wire. A step that
	// decided a pair the explanation could not describe names itself here.
	name string
	// decides reports whether this step separates the pair, and if so whether a
	// sorts first. A step that does not decide leaves the pair to the next one.
	decides func(a, b ranked) (decided, aFirst bool)
	// explain describes the decision this step just made. Called ONLY when
	// decides answered true for the same pair, so it never has to say "nothing".
	explain func(a, b ranked) crmcontracts.WorklistComparison
}

// rankSteps is the ordering, most decisive first.
//
// CROWDING LEADS, and it is the one thing that sorts above the level. A hundred
// unanswered customers are all genuinely level 1, so comparing levels first
// puts every one of them above the reader's overdue task and the page becomes a
// single kind of work again. The few that lead keep their level's precedence;
// the rest wait behind the other kinds — still level 1, still saying a buyer
// wrote last, just not all at once.
var rankSteps = []rankStep{
	{
		name: "pin",
		// A pin is a level difference and orders as one, but it is named
		// separately because "level" would hide the only override a reader has.
		decides: func(a, b ranked) (bool, bool) {
			if (a.item.Level == levelPinned) == (b.item.Level == levelPinned) {
				return false, false
			}
			return true, a.item.Level == levelPinned
		},
		explain: func(ranked, ranked) crmcontracts.WorklistComparison {
			return crmcontracts.WorklistComparison{Comparator: crmcontracts.WorklistComparisonComparatorPin}
		},
	},
	{
		name: "band",
		// The heading sorts ABOVE crowding, which is what makes each band one
		// contiguous run — and contiguity is the whole of what a heading is.
		// With crowding first, the ninth waiting customer sorted below the
		// hygiene rows while still banding as `now`, so the page drew `now`
		// twice with somebody else's work in between.
		//
		// Crowding still applies, WITHIN the band: past the lead group a wait
		// sits at the foot of its own heading rather than at the foot of the
		// page. That keeps the anti-monopoly rule the crowding exists for — a
		// hundred replies cannot own the band — without letting it move a row
		// out from under the heading that describes it.
		decides: func(a, b ranked) (bool, bool) {
			ai, bi := bandRank(bandOfRow(a)), bandRank(bandOfRow(b))
			return ai != bi, ai < bi
		},
		// A band difference has two possible causes and the reader needs the
		// right one. CROWDING is named where it is what moved the row: the
		// levels can be identical — two waiting customers, one of them the
		// ninth — and reporting "level 1 against level 1" would be a reason
		// nobody can check. Otherwise the level differs and says so.
		explain: func(a, b ranked) crmcontracts.WorklistComparison {
			if a.crowded != b.crowded && a.item.Level == b.item.Level {
				return crmcontracts.WorklistComparison{
					Comparator: crmcontracts.WorklistComparisonComparatorCrowded,
				}
			}
			return crmcontracts.WorklistComparison{
				Comparator: crmcontracts.WorklistComparisonComparatorLevel,
				Mine:       levelValue(a.item.Level),
				Theirs:     levelValue(b.item.Level),
			}
		},
	},
	{
		name:    "crowded",
		decides: func(a, b ranked) (bool, bool) { return a.crowded != b.crowded, b.crowded },
		// No values. "Mine 8th, theirs 9th" would be a number about the lane
		// rather than about either row, and the fact the reader needs is the
		// whole of it: the row below is one of many of its kind.
		explain: func(ranked, ranked) crmcontracts.WorklistComparison {
			return crmcontracts.WorklistComparison{Comparator: crmcontracts.WorklistComparisonComparatorCrowded}
		},
	},
	{
		name: "level",
		decides: func(a, b ranked) (bool, bool) {
			return a.item.Level != b.item.Level, a.item.Level < b.item.Level
		},
		explain: func(a, b ranked) crmcontracts.WorklistComparison {
			return crmcontracts.WorklistComparison{
				Comparator: crmcontracts.WorklistComparisonComparatorLevel,
				Mine:       levelValue(a.item.Level),
				Theirs:     levelValue(b.item.Level),
			}
		},
	},
	{
		name:    "deadline",
		decides: byDeadline,
		// The dates travel only when they READ differently. byDeadline can
		// decide on overdue-versus-not while both instants render as the same
		// minute, and "16:21 against 16:21" is a reason nobody can check — the
		// live page printed exactly that. When the date DID decide but reads
		// alike, the row says so plainly rather than falling through to a
		// comparator that decided nothing.
		explain: func(a, b ranked) crmcontracts.WorklistComparison {
			if sameMinute(a.deadlineAt, b.deadlineAt) {
				return crmcontracts.WorklistComparison{
					Comparator: crmcontracts.WorklistComparisonComparatorDeadline,
				}
			}
			return crmcontracts.WorklistComparison{
				Comparator: crmcontracts.WorklistComparisonComparatorDeadline,
				Mine:       dateValue(a),
				Theirs:     dateValue(b),
			}
		},
	},
	{
		name:    "expected_revenue",
		decides: byExpected,
		explain: func(a, b ranked) crmcontracts.WorklistComparison {
			return crmcontracts.WorklistComparison{
				Comparator: crmcontracts.WorklistComparisonComparatorExpectedRevenue,
				Mine:       moneyValue(a),
				Theirs:     moneyValue(b),
			}
		},
	},
	{
		name: "waiting_days",
		// Ordered on the BOUNDED age and reported as the TRUE one: the bounded
		// value is what decided, and the true value is what the row itself says
		// and a reader can check. Where the ceiling clipped both sides they tie
		// here and the next step decides, so no row claims an age decided it
		// once the ages had stopped mattering.
		decides: func(a, b ranked) (bool, bool) {
			return a.waitingRank != b.waitingRank, a.waitingRank > b.waitingRank
		},
		explain: func(a, b ranked) crmcontracts.WorklistComparison {
			return crmcontracts.WorklistComparison{
				Comparator: crmcontracts.WorklistComparisonComparatorWaitingDays,
				Mine:       daysValue(a.waitingDays),
				Theirs:     daysValue(b.waitingDays),
			}
		},
	},
	{
		name:    "relationship",
		decides: func(a, b ranked) (bool, bool) { return a.strength != b.strength, a.strength > b.strength },
		explain: func(ranked, ranked) crmcontracts.WorklistComparison {
			return crmcontracts.WorklistComparison{
				Comparator: crmcontracts.WorklistComparisonComparatorRelationship,
			}
		},
	},
	{
		name: "occurrence",
		decides: func(a, b ranked) (bool, bool) {
			return !a.occurredAt.Equal(b.occurredAt), a.occurredAt.Before(b.occurredAt)
		},
		// Named as waiting_days, because that is the word the contract has for
		// "the older one leads" and the client already draws it. The contract's
		// `order` means every step tied, and reporting it here would tell the
		// reader something false about their own page.
		//
		// The instants travel only when they read differently: thirteen seconds
		// apart renders as "23:20 against 23:20" under a heading about waiting
		// days, which is two wrong things at once.
		explain: func(a, b ranked) crmcontracts.WorklistComparison {
			if sameMinute(a.occurredAt, b.occurredAt) {
				return crmcontracts.WorklistComparison{
					Comparator: crmcontracts.WorklistComparisonComparatorWaitingDays,
				}
			}
			return crmcontracts.WorklistComparison{
				Comparator: crmcontracts.WorklistComparisonComparatorWaitingDays,
				Mine:       occurredValue(a),
				Theirs:     occurredValue(b),
			}
		},
	},
}
