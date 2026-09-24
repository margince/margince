// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// resultCache is the §6 result cache: workspace_id is part of the key
// (RT-AI-M7 — two tenants with identical inputs must never share an
// answer), TTL-bounded, with a per-workspace invalidation hook.
type resultCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]cacheEntry
}

type cacheEntry struct {
	workspaceID ids.WorkspaceID
	// generation names the Router binding that produced this answer, and it
	// is what makes clear a reclamation rather than the correctness step. A
	// completion in flight when a rebind lands finishes on the binding it
	// loaded and writes AFTER that clear, so the clear alone cannot keep the
	// old binding's words out of the new binding's answers.
	generation uint64
	resp       model.Response
	tier       Tier
	expires    time.Time
}

// maxResultCacheEntries bounds resident memory: expired entries are only
// reaped lazily on same-key reads, so without a cap a stream of unique
// requests would leave dead entries resident for the life of the process.
const maxResultCacheEntries = 1024

func newResultCache(ttl time.Duration) *resultCache {
	return &resultCache{ttl: ttl, now: time.Now, entries: map[string]cacheEntry{}}
}

func (c *resultCache) get(key string, wsID ids.WorkspaceID, generation uint64) (model.Response, Tier, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || c.now().After(entry.expires) {
		delete(c.entries, key)
		return model.Response{}, "", false
	}
	// Defense in depth for RT-AI-M7: even a corrupted key can never
	// serve another workspace's answer.
	if entry.workspaceID != wsID {
		return model.Response{}, "", false
	}
	// An answer belongs to the binding that produced it. Serving one across a
	// rebind would report a previous model's words under the provider and
	// model identity of the one now bound, and that identity decides whether a
	// caller is told "no AI provider is configured" or "the model answered
	// badly" — two different things for an operator to do.
	if entry.generation != generation {
		return model.Response{}, "", false
	}
	return entry.resp, entry.tier, true
}

// put keeps one completion for replay, unless it is not an answer worth
// replaying. The rule lives here rather than at a call site so that no writer
// can cache what it forgot to check.
func (c *resultCache) put(key string, wsID ids.WorkspaceID, generation uint64, resp model.Response, tier Tier) {
	if !replayable(resp) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= maxResultCacheEntries {
		c.makeRoomLocked()
	}
	c.entries[key] = cacheEntry{workspaceID: wsID, generation: generation, resp: resp, tier: tier, expires: c.now().Add(c.ttl)}
}

// replayable reports whether a completion may be served again to the next
// identical request. A reply cut off at the output ceiling, or one carrying no
// text at all, is one bad roll of the model: cached, it becomes every identical
// request's answer for the TTL, where a fresh call might have finished. Only
// the cut-off terminal is refused, because every other terminal a Response
// can carry is spelled per wire ("stop", "end_turn", "STOP") and a finished
// reply must stay cacheable on all of them.
func replayable(resp model.Response) bool {
	return resp.FinishReason != model.FinishReasonLength && strings.TrimSpace(resp.Text) != ""
}

// forget drops one request's cached completion. The structured-output
// pipeline calls this when a response fails validation: an invalid answer
// must never be replayed to a future identical request — the retry's whole
// value is a fresh roll of the model.
func (c *resultCache) forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// makeRoomLocked frees at least one slot: first a full sweep of expired
// entries (the only global reap — get only deletes the key it reads),
// then, if every entry is still live, the soonest-to-expire one goes —
// it holds the least remaining TTL value.
func (c *resultCache) makeRoomLocked() {
	now := c.now()
	for key, entry := range c.entries {
		if now.After(entry.expires) {
			delete(c.entries, key)
		}
	}
	if len(c.entries) < maxResultCacheEntries {
		return
	}
	var soonestKey string
	var soonest time.Time
	for key, entry := range c.entries {
		if soonestKey == "" || entry.expires.Before(soonest) {
			soonestKey, soonest = key, entry.expires
		}
	}
	delete(c.entries, soonestKey)
}

// clear drops every cached answer, whatever workspace produced it. A rebind
// calls it to reclaim the memory: each entry was produced by a model binding
// that no longer exists, so none of them can ever be read again, and left
// resident they would evict live entries through the size cap. Keeping them
// OUT of the new binding's answers is the generation stamp's job, not this
// one's — a call already in flight writes after this runs.
func (c *resultCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
}

func (c *resultCache) invalidate(wsID ids.WorkspaceID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if entry.workspaceID == wsID {
			delete(c.entries, key)
		}
	}
}
