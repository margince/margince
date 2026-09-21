// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"sync"
	"time"
)

// localStore counts in this process's memory. It is what a Limiter uses until
// a shared store is bound, and it is per-Limiter rather than per-Registry: two
// limiters that happen to carry the same name must not share buckets in a test
// binary, where the same name is constructed once per case.
type localStore struct {
	mu      sync.Mutex
	windows map[string]window
	sweepAt time.Time
}

// window is one key's current fixed window: when it opened, how many events
// have landed in it, and how long it runs. The span is held per entry rather
// than per store because the sweep below walks every key at once and must
// judge each against the span it was opened with.
type window struct {
	start time.Time
	count int
	span  time.Duration
}

func newLocalStore() *localStore { return &localStore{windows: make(map[string]window)} }

func (s *localStore) count(key string, span time.Duration, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now, span)

	w, ok := s.windows[key]
	if !ok || now.Sub(w.start) >= w.span {
		w = window{start: now, span: span}
	}
	w.count++
	s.windows[key] = w
	return w.count, nil
}

func (s *localStore) peek(key string, _ time.Duration, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.windows[key]
	if !ok || now.Sub(w.start) >= w.span {
		return 0, nil
	}
	return w.count, nil
}

// forget drops every bucket. The prefix is ignored because a local store
// belongs to exactly one Limiter, so its whole contents are that Limiter's.
func (s *localStore) forget(string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windows = make(map[string]window)
	// sweepAt goes too: a future deadline left behind would keep the next
	// sweep from running on a map that no longer matches it.
	s.sweepAt = time.Time{}
	return nil
}

// sweep drops expired windows so abandoned keys do not accumulate forever —
// an unauthenticated endpoint sees arbitrary keys. Amortized: at most once
// per span, on a call that was already taking the lock.
func (s *localStore) sweep(now time.Time, span time.Duration) {
	if !now.After(s.sweepAt) {
		return
	}
	for k, w := range s.windows {
		if now.Sub(w.start) >= w.span {
			delete(s.windows, k)
		}
	}
	s.sweepAt = now.Add(span)
}
