// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// BackfillTuning sizes one history page and its fetching. A zero field keeps
// the connector's own value, so an installation that sets nothing runs as it
// always did.
type BackfillTuning struct {
	// PageSize is how many messages one page lists.
	PageSize int
	// FetchWorkers is how many messages one page downloads at once.
	FetchWorkers int
	// Lookahead is how far downloads may run ahead of the saved messages.
	// Zero means twice the workers.
	Lookahead int
}

// Resolve answers the page size, fetch workers and look-ahead to use. The
// connector's own page size and workers stand in for a field left zero. The
// look-ahead never falls below the workers, or the extra workers would sit idle.
func (t BackfillTuning) Resolve(pageSize, workers int) (page, fetchWorkers, lookahead int) {
	page, fetchWorkers = pageSize, workers
	if t.PageSize > 0 {
		page = t.PageSize
	}
	if t.FetchWorkers > 0 {
		fetchWorkers = t.FetchWorkers
	}
	lookahead = 2 * fetchWorkers
	if t.Lookahead > 0 {
		lookahead = t.Lookahead
	}
	return page, fetchWorkers, max(lookahead, fetchWorkers)
}
