// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Pure mechanics behind GET /organizations/{id}/hierarchy-rollup
// (RD-T04): the RBAC-aware BFS prune over the parent→children org
// graph, calendar-quarter bounds in the workspace timezone, and the
// two rounding rules (win-weighted value, FX base conversion) the
// rollup's totals are built from. No DB and no HTTP live here — the
// gated tree walk and measures are in orgrollupread.go, the HTTP
// handler arrives with the transport slice.

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// orgTreeNode is one row of the flattened organization hierarchy:
// enough to walk parent→children and to name a node the caller can't
// see into.
type orgTreeNode struct {
	id          ids.UUID
	parentID    *ids.UUID
	displayName string
}

// restrictedNode is a hierarchy node the caller cannot read, disclosed
// by identity and name only — never by its figures, and never by its
// subtree's.
type restrictedNode struct {
	ID          ids.UUID
	DisplayName string
}

// pruneUnreadable walks the org tree breadth-first from rootID over the
// parent→children adjacency nodes encodes, splitting it into the
// RBAC-readable set the rollup sums and the restricted set it discloses
// without summing. The root itself is never disclosed as restricted —
// an unreadable root is a 404 at the HTTP layer (rootReadable=false
// signals that), not a member of any list.
//
// A node the caller can't read is the deepest point a branch is
// visited: its children are never inspected, so a grandchild behind a
// restricted node is neither included nor separately disclosed. Because
// readable is consulted fresh for every node, a live grant that flips a
// node back to readable pulls its whole readable subtree back in on the
// very next call — pruneUnreadable holds no memory of a prior result.
func pruneUnreadable(rootID ids.UUID, nodes []orgTreeNode, readable func(ids.UUID) bool) (included []ids.UUID, restricted []restrictedNode, rootReadable bool) {
	included = []ids.UUID{}
	restricted = []restrictedNode{}
	if !readable(rootID) {
		return included, restricted, false
	}

	childrenByParent := make(map[ids.UUID][]orgTreeNode, len(nodes))
	for _, n := range nodes {
		if n.parentID == nil {
			continue
		}
		childrenByParent[*n.parentID] = append(childrenByParent[*n.parentID], n)
	}

	included = append(included, rootID)
	queue := []ids.UUID{rootID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range childrenByParent[current] {
			if !readable(child.id) {
				restricted = append(restricted, restrictedNode{ID: child.id, DisplayName: child.displayName})
				continue
			}
			included = append(included, child.id)
			queue = append(queue, child.id)
		}
	}
	return included, restricted, true
}

// currentQuarterBounds returns the quarter [start, end) that now falls in,
// evaluated in loc — the workspace timezone, not UTC, so a moment shortly
// after midnight UTC can still belong to the prior quarter (and year) for a
// workspace west of Greenwich.
//
// fiscalStartMonth is the month the installation's business year begins, 1..12.
// The quarters are cut FROM it rather than from January, so an installation
// whose year starts in April reads April–June as its first quarter — the same
// cut the period reports make, and the figure on the company page has to agree
// with the report a reader checks it against.
//
// January (the default, and every installation that predates the setting)
// leaves the offset at zero and reproduces the calendar quarters exactly.
func currentQuarterBounds(now time.Time, loc *time.Location, fiscalStartMonth int) (start, end time.Time) {
	local := now.In(loc)
	// Months since the fiscal year began, 0..11. The +12 keeps the modulo
	// positive for a month earlier in the calendar than the fiscal start —
	// February under an April start is 10 months in, not -2.
	monthsIn := (int(local.Month()) - fiscalStartMonth + 12) % 12
	quarterOffset := (monthsIn / 3) * 3
	// Anchored on the fiscal start month IN local's own calendar year, then
	// walked forward by whole quarters. That anchor can land in the future —
	// February under an April start sits in the fiscal year that began the
	// PREVIOUS April — so a start after `local` is pulled back a year.
	start = time.Date(local.Year(), time.Month(fiscalStartMonth), 1, 0, 0, 0, 0, loc).
		AddDate(0, quarterOffset, 0)
	if start.After(local) {
		start = start.AddDate(-1, 0, 0)
	}
	end = start.AddDate(0, 3, 0)
	return start, end
}

// FXRateUnavailableError reports that the rollup needed a stored FX
// rate for currency as of a point in time and none was on file — the
// system never invents a rate=1 fallback, per formulas §11. Exported so
// the HTTP layer and the integration suites match it via errors.As and
// map it to 422 fx_rate_unavailable.
type FXRateUnavailableError struct {
	Currency string
	AsOf     time.Time
}

// Error names the missing rate and what to load, on a POINTER receiver to match
// MessageFault below. Split receivers would let a `return FXRateUnavailableError{…}`
// without the & compile, satisfy error, and be missed by every errors.As in the
// tree — turning a governed 422 into an opaque 500.
func (e *FXRateUnavailableError) Error() string {
	return fmt.Sprintf("no stored FX rate for %s as of %s; record today's rate for %s before retrying the rollup",
		e.Currency, e.AsOf.Format(time.DateOnly), e.Currency)
}

// MessageFault carries the 422 on the error itself, so the taxonomy answers it
// on any surface and not only where orgrollup_handlers runs.
//
// MessageFault is the whole point here rather than an incidental choice: the
// missing rate is workspace data, not an argument, and this is the exact case
// the fault-form doc names — an agent told to fix `fx_rate_to_base` on a call
// that has no such argument will either retry unchanged or invent a value. The
// message says what is absent and who must load it.
func (e *FXRateUnavailableError) MessageFault() (code, message string) {
	return "fx_rate_unavailable", e.Error()
}
