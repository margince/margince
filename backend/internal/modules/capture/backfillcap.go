// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// How much history an installation is willing to take.
//
// The offered set (backfill.go) is the product's: 3 months to 10 years, pinned
// to the contract enum and the table's CHECK so a picker cannot offer a window
// the database refuses. The CAP is the installation's, and it answers a
// different question — not "which windows exist" but "how far back may anybody
// here reach".
//
// Without one the decision belongs to whoever connects a mailbox, and it is
// made again every time. A ten-year import is not a bigger version of a
// one-year import: it predates every notice, policy and exclusion rule the
// installation has handed out, and it is the input to the raw_capture growth
// the retention rule then has to age back out.
//
// UNSET MEANS UNCAPPED, which is the behaviour every existing installation
// already has. The ceiling was widened deliberately once (a mailbox with a
// decade of history is a real customer), so a default cap here would quietly
// take that back from installations that chose it.

// maxBackfillMonths is the installation's ceiling, or 0 for none.
//
// Held on the Registry rather than read from config at each call: the enumerate
// -only constructions build a Registry with no deployment config at all, and a
// ceiling read per call would have to answer for them too.
func (r *Registry) capOrUnbounded() int { return r.maxBackfillMonths }

// WithMaxBackfillMonths sets the installation's ceiling and returns the
// registry, in the shape every other builder on this type takes — it carries a
// mutex, so it is configured in place rather than copied.
//
// Zero or a negative is no cap, which is what an enumerate-only construction
// and every deployment that has not set one pass.
func (r *Registry) WithMaxBackfillMonths(months int) *Registry {
	if months > 0 {
		r.maxBackfillMonths = months
	}
	return r
}

// OfferedBackfillWindows is the supported set narrowed to what this
// installation admits, in reach order.
//
// The refusal below names this rather than the product's full set: an operator
// told "3, 6, 12, 24, 36, 60, 84, 120" by a build that accepts four of them has
// been told about somebody else's installation.
func (r *Registry) OfferedBackfillWindows() []int {
	all := BackfillWindowMonths()
	ceiling := r.capOrUnbounded()
	if ceiling <= 0 {
		return all
	}
	out := make([]int, 0, len(all))
	for _, m := range all {
		if m <= ceiling {
			out = append(out, m)
		}
	}
	return out
}

// admitsWindow reports whether months is one the product offers AND one this
// installation admits.
//
// Both halves, in that order: the cap narrows a set it does not define, so a
// ceiling set above the widest supported window still admits only the windows
// the contract and the table's CHECK have.
func (r *Registry) admitsWindow(months int) bool {
	if !backfillWindows[months] {
		return false
	}
	ceiling := r.capOrUnbounded()
	return ceiling <= 0 || months <= ceiling
}
