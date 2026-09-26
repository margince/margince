// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"sync"
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
