// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"sync"
	"time"
)

// catalogueCache holds one vendor's parsed public list for catalogueCacheTTL.
// Only a successful read is kept, so a failure is retried on the next ask.
type catalogueCache[T any] struct {
	fetcher CatalogueFetcher
	clock   Clock
	parse   func([]byte) (T, error)

	mu        sync.Mutex
	cached    T
	held      bool
	fetchedAt time.Time
}

func (c *catalogueCache[T]) fresh(ctx context.Context) (T, error) {
	c.mu.Lock()
	cached, held, fetchedAt := c.cached, c.held, c.fetchedAt
	c.mu.Unlock()
	if held && c.clock.Now().Sub(fetchedAt) < catalogueCacheTTL {
		return cached, nil
	}
	var zero T
	body, err := c.fetcher.Fetch(ctx)
	if err != nil {
		return zero, err
	}
	parsed, err := c.parse(body)
	if err != nil {
		return zero, err
	}
	c.mu.Lock()
	c.cached, c.held, c.fetchedAt = parsed, true, c.clock.Now()
	c.mu.Unlock()
	return parsed, nil
}
