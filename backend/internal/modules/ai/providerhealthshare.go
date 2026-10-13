// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"log/slog"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	// shareTimeout bounds one Redis round trip made for the shared status.
	shareTimeout = 2 * time.Second
	// shareRefresh re-publishes a status that is still unhealthy but has not
	// changed, well inside the store's expiry, so a long degraded spell is not
	// forgotten while it is still true.
	shareRefresh = 10 * time.Minute
	// shareTick is how often a process with an unhealthy provider refreshes its
	// shared record and notices that another process cleared it.
	shareTick = 30 * time.Second
	// shareSettle is how old a published status must be before its absence from
	// the store is read as "cleared by someone" rather than "not written yet".
	shareSettle = time.Minute
)

// ProviderHealthStore is where processes leave each provider's unhealthy
// status for the others to read. The api serves the operator surfaces but the
// lanes that meet an outage mostly run in the worker, so a per-process book
// alone would answer "all providers are answering" for a worker-only outage.
//
// Sharing is for display only: no process brakes on another's status.
type ProviderHealthStore interface {
	Publish(ctx context.Context, provider string, status model.ProviderHealthStatus) error
	Clear(ctx context.Context, provider string) error
	Load(ctx context.Context) (map[string]model.ProviderHealthStatus, error)
}

// ShareProviderHealth makes this process publish its provider statuses to store
// and read the others' through it. A role without a shared store never calls it
// and keeps answering for itself alone.
func ShareProviderHealth(ctx context.Context, store ProviderHealthStore) {
	sharedProviderHealth.sharer.use(store)
	go sharedProviderHealth.maintain(ctx, shareTick)
}

// maintain keeps this process's records honest for as long as it runs: an
// unhealthy status nobody touches is republished before the store expires it,
// and a status another process cleared (a confirmed key test, a rebind) is
// cleared here too, so the fix reaches the process that owns the retry window.
// It costs nothing while every provider is healthy.
func (b *providerBook) maintain(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.reconcile(ctx)
		}
	}
}

func (b *providerBook) reconcile(ctx context.Context) {
	b.mu.Lock()
	trackers := make(map[string]*providerTracker, len(b.trackers))
	maps.Copy(trackers, b.trackers)
	b.mu.Unlock()
	unhealthy := false
	for _, t := range trackers {
		if t.refreshIfDue() {
			unhealthy = true
		}
	}
	if !unhealthy {
		return
	}
	shared, ok := b.sharer.loadChecked(ctx)
	if !ok {
		return
	}
	for name, t := range trackers {
		if _, held := shared[name]; !held {
			t.resetIfClearedElsewhere()
		}
	}
}

// healthSharer pushes status changes to the store off the call path: a change
// is queued, one goroutine at a time drains the queue, and a provider changing
// twice before the drain reaches it costs one write of the latest status.
// The model call therefore never waits on Redis, and the queue is bounded by
// the number of providers.
type healthSharer struct {
	mu      sync.Mutex
	store   ProviderHealthStore
	pending map[string]model.ProviderHealthStatus
	running bool
	// writeFailing and readFailing make a run of Redis faults one log line, not
	// one per call.
	writeFailing, readFailing bool
}

func (s *healthSharer) use(store ProviderHealthStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

// enqueue records provider's latest status; an OK status clears the shared key.
func (s *healthSharer) enqueue(provider string, st model.ProviderHealthStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return
	}
	if s.pending == nil {
		s.pending = map[string]model.ProviderHealthStatus{}
	}
	s.pending[provider] = st
	if !s.running {
		s.running = true
		go s.drain()
	}
}

func (s *healthSharer) drain() {
	for {
		s.mu.Lock()
		batch, store := s.pending, s.store
		s.pending = nil
		if len(batch) == 0 || store == nil {
			s.running = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		for provider, st := range batch {
			s.write(store, provider, st)
		}
	}
}

func (s *healthSharer) write(store ProviderHealthStore, provider string, st model.ProviderHealthStatus) {
	ctx, cancel := context.WithTimeout(context.Background(), shareTimeout)
	defer cancel()
	var err error
	if st.Health == model.HealthOK {
		err = store.Clear(ctx, provider)
	} else {
		err = store.Publish(ctx, provider, st)
	}
	s.mu.Lock()
	first := err != nil && !s.writeFailing
	recovered := err == nil && s.writeFailing
	s.writeFailing = err != nil
	s.mu.Unlock()
	switch {
	case first:
		slog.WarnContext(ctx, "the shared AI provider status could not be written; other processes may not see it",
			"provider", provider, "error", err)
	case recovered:
		slog.InfoContext(ctx, "the shared AI provider status is writable again")
	}
}

// loadChecked is load that also says whether the store answered, because an
// empty answer and an unreachable store mean opposite things to reconcile.
func (s *healthSharer) loadChecked(ctx context.Context) (map[string]model.ProviderHealthStatus, bool) {
	s.mu.Lock()
	store := s.store
	s.mu.Unlock()
	if store == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, shareTimeout)
	defer cancel()
	shared, err := store.Load(ctx)
	return shared, err == nil
}

// load reads what every process published; nil when nothing is shared or Redis
// cannot be reached, which the caller answers from its own book.
func (s *healthSharer) load(ctx context.Context) map[string]model.ProviderHealthStatus {
	s.mu.Lock()
	store := s.store
	s.mu.Unlock()
	if store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, shareTimeout)
	defer cancel()
	shared, err := store.Load(ctx)
	s.mu.Lock()
	first := err != nil && !s.readFailing
	recovered := err == nil && s.readFailing
	s.readFailing = err != nil
	s.mu.Unlock()
	switch {
	case first:
		slog.WarnContext(ctx, "the shared AI provider status could not be read; answering for this process alone",
			"error", err)
	case recovered:
		slog.InfoContext(ctx, "the shared AI provider status is readable again")
	}
	return shared
}

// report lists the providers that are not OK in this process or in any other
// that shares its status, by name.
func (b *providerBook) report(ctx context.Context) []ProviderHealthEntry {
	return mergeProviderHealth(b.snapshot(), b.sharer.load(ctx)) //nolint:contextcheck // snapshot reads memory; the Redis write it may queue runs on the drain goroutine, which owns its own deadline by design
}

// mergeProviderHealth keeps one entry per provider. A blocking status beats a
// degraded one, whichever process saw it, because that is the one an operator
// must act on; between two of a kind the more recent fault wins, and a tie
// goes to the local view.
func mergeProviderHealth(local []ProviderHealthEntry, shared map[string]model.ProviderHealthStatus) []ProviderHealthEntry {
	best := make(map[string]model.ProviderHealthStatus, len(local)+len(shared))
	for _, e := range local {
		best[e.Provider] = e.Status
	}
	for provider, st := range shared {
		if st.Health == model.HealthOK {
			continue
		}
		if cur, ok := best[provider]; !ok || outranks(st, cur) {
			best[provider] = st
		}
	}
	names := make([]string, 0, len(best))
	for name := range best {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []ProviderHealthEntry
	for _, name := range names {
		out = append(out, ProviderHealthEntry{Provider: name, Status: best[name]})
	}
	return out
}

func outranks(a, b model.ProviderHealthStatus) bool {
	if a.Health.Blocking() != b.Health.Blocking() {
		return a.Health.Blocking()
	}
	return a.Since.After(b.Since)
}

// share queues the tracker's status for the other processes when it changed,
// or when an unchanged unhealthy status is due its refresh. Called with t.mu held.
func (t *providerTracker) share(before model.ProviderHealthStatus) {
	if t.notify == nil {
		return
	}
	changed := t.status.Health != before.Health ||
		!t.status.Since.Equal(before.Since) || !t.status.RetryAfter.Equal(before.RetryAfter)
	now := t.now()
	if !changed && (t.status.Health == model.HealthOK || now.Sub(t.sharedAt) < shareRefresh) {
		return
	}
	t.sharedAt = now
	t.notify(t.status)
}

// refreshIfDue republishes an unhealthy status that has sat unchanged for most
// of the store's expiry, and reports whether the tracker is unhealthy at all.
func (t *providerTracker) refreshIfDue() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status.Health == model.HealthOK {
		return false
	}
	if t.notify != nil && t.now().Sub(t.sharedAt) >= shareRefresh {
		t.sharedAt = t.now()
		t.notify(t.status)
	}
	return true
}

// resetIfClearedElsewhere recovers a blocked tracker whose shared record has
// gone: another process cleared it, which only an operator's fix does, and the
// record was old enough to have been written (else it is merely unpublished).
func (t *providerTracker) resetIfClearedElsewhere() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.status.Health.Blocking() || t.sharedAt.IsZero() || t.now().Sub(t.sharedAt) < shareSettle {
		return
	}
	before := t.status
	t.recover()
	t.share(before)
}
