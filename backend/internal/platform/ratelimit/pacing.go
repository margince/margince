// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Budget charges a weighted operation against one shared, smoothly paced key.
type Budget struct {
	Key       string
	PerMinute int
	Cost      int
}

// Pacer grants all budgets together or returns a wait without spending any.
// Unlike Allow, a refused probe never increases its eventual wait.
type Pacer struct {
	registry *Registry
	now      func() time.Time
}

type paceClock struct {
	mu      sync.Mutex
	next    map[string]int64
	sweepAt int64
}

// NewPacer uses this process's existing shared-store binding.
func NewPacer() *Pacer { return process.NewPacer(time.Now) }

// NewPacer accepts a clock for deterministic local quota fixtures.
func (r *Registry) NewPacer(now func() time.Time) *Pacer {
	return &Pacer{registry: r, now: now}
}

// Reserve grants all budgets or returns the longest wait and its scoped key.
func (p *Pacer) Reserve(ctx context.Context, budgets ...Budget) (time.Duration, string, error) {
	if err := ctx.Err(); err != nil {
		return 0, "", err
	}
	if len(budgets) == 0 {
		return 0, "", nil
	}
	keys := make([]string, 0, len(budgets))
	charges := make([]int64, 0, len(budgets))
	seen := make(map[string]bool, len(budgets))
	for _, b := range budgets {
		if b.Key == "" || !meterable(b.Key) || seen[b.Key] || b.PerMinute <= 0 || b.PerMinute > 1_000_000_000 || b.Cost <= 0 || b.Cost > b.PerMinute {
			return 0, "", errors.New("ratelimit: invalid weighted budget")
		}
		seen[b.Key] = true
		keys = append(keys, pacedKey(b.Key))
		charges = append(charges, (time.Minute.Microseconds()*int64(b.Cost)+int64(b.PerMinute)-1)/int64(b.PerMinute))
	}
	if s := p.registry.sharedStore(); s != nil {
		shared, ok := s.(*sharedStore)
		if !ok {
			return 0, "", errors.New("ratelimit: shared store cannot pace operations")
		}
		return shared.pace(ctx, keys, charges)
	}
	wait, key := p.registry.pacing.reserve(p.now().UnixMicro(), keys, charges)
	return wait, key, nil
}

func (c *paceClock) reserve(now int64, keys []string, charges []int64) (time.Duration, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var wait int64
	var blocked string
	if c.next == nil {
		c.next = make(map[string]int64)
	}
	if now >= c.sweepAt {
		for key, next := range c.next {
			if next <= now {
				delete(c.next, key)
			}
		}
		c.sweepAt = now + time.Minute.Microseconds()
	}
	for _, key := range keys {
		if delay := c.next[key] - now; delay > wait {
			wait = delay
			blocked = key
		}
	}
	if wait > 0 {
		return time.Duration(wait) * time.Microsecond, blocked
	}
	for i, key := range keys {
		c.next[key] = now + charges[i]
	}
	return 0, ""
}

var paceScript = redis.NewScript(`
local t=redis.call('TIME')
local now=tonumber(t[1])*1000000+tonumber(t[2])
local wait=0
local blocked=0
for i,key in ipairs(KEYS) do
 local next=tonumber(redis.call('GET',key)) or 0
 if next-now>wait then wait=next-now;blocked=i end
end
if wait>0 then return {wait,blocked} end
for i,key in ipairs(KEYS) do
 local charge=tonumber(ARGV[i])
 redis.call('SET',key,string.format('%.0f',now+charge),'PX',math.ceil(charge/1000))
end
return {0,0}`)

func (s *sharedStore) pace(ctx context.Context, keys []string, charges []int64) (time.Duration, string, error) {
	ctx, cancel := context.WithTimeout(ctx, sharedTimeout)
	defer cancel()
	args := make([]any, len(charges))
	for i, charge := range charges {
		args[i] = charge
	}
	wait, err := paceScript.Run(ctx, s.rdb, keys, args...).Int64Slice()
	if err != nil {
		return 0, "", fmt.Errorf("ratelimit: pacing store unavailable: %w", err)
	}
	if len(wait) != 2 || wait[1] < 0 || wait[1] > int64(len(keys)) {
		return 0, "", errors.New("ratelimit: invalid pacing response")
	}
	if wait[0] <= 0 {
		return 0, "", nil
	}
	if wait[1] == 0 {
		return 0, "", errors.New("ratelimit: pacing response has no scope")
	}
	return time.Duration(wait[0]) * time.Microsecond, keys[wait[1]-1], nil
}
