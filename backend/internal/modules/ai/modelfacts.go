// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"sync"
	"time"
)

// perModelFacts remembers what a server said about each model, so the question
// costs one request per model per client rather than one per call. Its
// lifetime is the client's, which is the binding's: a routing rebind builds a
// new client. The zero value is ready to use.
type perModelFacts[V any] struct {
	mu      sync.Mutex
	byModel map[string]V
}

// lookup returns model's fact, asking for it once. A failed ask is not
// remembered, so the next call asks again.
//
// The lock guards the map alone. An ask is an HTTP call, and holding the lock
// across it would make every call on the client wait for one model's answer.
// Two concurrent first calls may both ask; the answers are equal.
func (p *perModelFacts[V]) lookup(ctx context.Context, model string, ask func(context.Context, string) (V, error)) (V, error) {
	p.mu.Lock()
	cached, ok := p.byModel[model]
	p.mu.Unlock()
	if ok {
		return cached, nil
	}
	fact, err := ask(ctx, model)
	if err != nil {
		return fact, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.byModel == nil {
		p.byModel = map[string]V{}
	}
	p.byModel[model] = fact
	return fact, nil
}

// catalogRetryAfter is how long a failed catalog read stands before the next
// call asks again: a broker that is down costs one wait per window, never one
// per call.
const catalogRetryAfter = time.Minute

// catalogFact is what a server said about every model at once — a broker's
// catalog — asked once per client however many first calls arrive together.
// A success lasts the client's lifetime, as perModelFacts' answers do. The zero
// value is not ready: now must be set.
type catalogFact[V any] struct {
	now func() time.Time

	mu    sync.Mutex
	asked *catalogAsk[V]
}

// catalogAsk is one read of the catalog; done closes when it settles.
type catalogAsk[V any] struct {
	done    chan struct{}
	value   V
	err     error
	settled time.Time
}

// get answers the catalog, joining a read already in flight. The read runs
// detached from ctx, bounded by ask's own timeout, so one caller giving up
// neither fails the others waiting on it nor poisons the remembered answer.
func (f *catalogFact[V]) get(ctx context.Context, ask func(context.Context) (V, error)) (V, error) {
	f.mu.Lock()
	current := f.asked
	if current == nil || f.stale(current) {
		current = &catalogAsk[V]{done: make(chan struct{})}
		f.asked = current
		go current.run(context.WithoutCancel(ctx), ask, f.now)
	}
	f.mu.Unlock()
	select {
	case <-current.done:
		return current.value, current.err
	case <-ctx.Done():
		var none V
		return none, ctx.Err()
	}
}

// stale says a settled read failed long enough ago to be asked again; a read
// in flight is never stale, so it is joined rather than repeated.
func (f *catalogFact[V]) stale(asked *catalogAsk[V]) bool {
	select {
	case <-asked.done:
		return asked.err != nil && f.now().Sub(asked.settled) >= catalogRetryAfter
	default:
		return false
	}
}

func (a *catalogAsk[V]) run(ctx context.Context, ask func(context.Context) (V, error), now func() time.Time) {
	defer close(a.done)
	a.value, a.err = ask(ctx)
	a.settled = now()
}
