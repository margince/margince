// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"sync"
	"time"
)

// DailyFullPass remembers the UTC day this process last finished a full drift
// pass. It is not stored: a restart or a second worker costs one extra full
// pass, never a missed one.
type DailyFullPass struct {
	mu   sync.Mutex
	done string
}

// Scope is the drift scope of a pass starting at now.
func (d *DailyFullPass) Scope(now time.Time) DriftScope {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.done == now.UTC().Format(time.DateOnly) {
		return DriftRecent
	}
	return DriftFull
}

// Finished records a full pass that started at now and completed.
func (d *DailyFullPass) Finished(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.done = now.UTC().Format(time.DateOnly)
}
