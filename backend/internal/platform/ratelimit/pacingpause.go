// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

func pacedKey(key string) string { return "ratelimit:pace:{provider}:" + key }

// Pause extends a provider's shared deadline without shortening an existing wait.
func (p *Pacer) Pause(ctx context.Context, key string, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" || !meterable(key) || delay <= 0 {
		return errors.New("ratelimit: invalid pacing pause")
	}
	key = pacedKey(key)
	if s := p.registry.sharedStore(); s != nil {
		shared, ok := s.(*sharedStore)
		if !ok {
			return errors.New("ratelimit: shared store cannot pause operations")
		}
		ctx, cancel := context.WithTimeout(ctx, sharedTimeout)
		defer cancel()
		return pauseScript.Run(ctx, shared.rdb, []string{key}, delay.Microseconds()).Err()
	}
	c := &p.registry.pacing
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.next == nil {
		c.next = make(map[string]int64)
	}
	c.next[key] = max(c.next[key], p.now().Add(delay).UnixMicro())
	return nil
}

var pauseScript = redis.NewScript(`
local t=redis.call('TIME')
local now=tonumber(t[1])*1000000+tonumber(t[2])
local deadline=now+tonumber(ARGV[1])
local previous=tonumber(redis.call('GET',KEYS[1])) or 0
if deadline>previous then
 redis.call('SET',KEYS[1],string.format('%.0f',deadline),'PX',math.ceil((deadline-now)/1000))
end
return 0`)
