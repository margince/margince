// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"sync"
	"time"
)

// seededOccurrences is the per-day memo of (spec, trigger_ref) pairs whose
// EnqueueJob returned nil. It is only a shortcut: the insert stays ON CONFLICT
// DO NOTHING, so a restart or a second worker still seeds each pair once.
type seededOccurrences struct {
	mu    sync.Mutex
	day   string
	pairs map[[2]string]struct{}
}

// has reports whether the pair was seeded today, dropping a previous day's
// pairs first so the memo never outgrows one day of seats.
func (m *seededOccurrences) has(now time.Time, spec, triggerRef string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if day := now.UTC().Format(time.DateOnly); day != m.day {
		m.day, m.pairs = day, map[[2]string]struct{}{}
	}
	_, ok := m.pairs[[2]string{spec, triggerRef}]
	return ok
}

// add records a pair after its insert succeeded; has runs first and has
// already made the map for this day.
func (m *seededOccurrences) add(spec, triggerRef string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pairs[[2]string{spec, triggerRef}] = struct{}{}
}
