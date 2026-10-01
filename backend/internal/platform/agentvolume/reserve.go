// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// reserveScript charges n only if the window can still pay for all of it, and
// answers {admitted, observed, limit}. Reading the balance and charging it in
// one script is the point: two changes that each read the same remainder and
// then charged it would both be admitted. The limit is effectiveLimit's, spelled
// in Lua because it has to be read in the same step.
// KEYS=[count, released] ARGV=[n, base limit, ttlSeconds].
var reserveScript = redis.NewScript(`
local n = tonumber(ARGV[1])
local base = tonumber(ARGV[2])
local observed = tonumber(redis.call('GET', KEYS[1]) or '0')
local released = tonumber(redis.call('GET', KEYS[2]) or '0')
local limit = base
if released > 0 then limit = base * (released + 1) end
if observed + n > limit then return {0, observed, limit} end
local total = redis.call('INCRBY', KEYS[1], n)
if total == n then redis.call('EXPIRE', KEYS[1], tonumber(ARGV[3])) end
return {1, total, limit}`)

// Reserve charges n against ctx's agent on c when the window can pay for all n,
// and refuses without charging anything when it cannot. A caller outside the
// control is admitted and charged nothing; an agent whose counter cannot be
// reached is refused, for the reason Read fails closed.
func (m *Meter) Reserve(ctx context.Context, c Counter, n int) (Reading, bool, error) {
	bucket := m.Bucket()
	if n <= 0 || m.unbounded || !governed(ctx) {
		return Reading{Counter: c, Limit: m.limits.of(c), Allowance: m.limits.of(c), Bucket: bucket}, true, nil
	}
	ws, agent, named := callerKey(ctx)
	if !named || m.rdb == nil {
		return Reading{Counter: c, Limit: m.limits.of(c), Allowance: m.limits.of(c), Exceeded: true, Bucket: bucket}, false, nil
	}
	answer, err := reserveScript.Run(ctx, m.rdb,
		[]string{m.countKey(ws, agent, c, bucket), m.releaseKey(ws, agent, c, bucket)},
		n, m.limits.of(c), m.ttlSeconds()).Int64Slice()
	m.noteReach(err)
	if err != nil {
		return Reading{}, false, fmt.Errorf("agentvolume: reserving %d on %s: %w", n, c, err)
	}
	if len(answer) != 3 {
		return Reading{}, false, fmt.Errorf("agentvolume: reserving on %s answered %d values, want 3", c, len(answer))
	}
	admitted := answer[0] == 1
	reading := Reading{
		Counter: c, Observed: int(answer[1]), Limit: int(answer[2]), Allowance: m.limits.of(c),
		Exceeded: !admitted, Bucket: bucket,
	}
	return reading, admitted, nil
}

// Refund gives back n a reservation took for a change that did not commit.
// bucket is the window the reservation was charged to, so a refund landing
// after the window rolled returns the units to the window that paid them.
func (m *Meter) Refund(ctx context.Context, c Counter, n int, bucket int64) error {
	if n <= 0 || m.unbounded || !governed(ctx) {
		return nil
	}
	ws, agent, named := callerKey(ctx)
	if !named || m.rdb == nil {
		return nil
	}
	if err := m.rdb.DecrBy(ctx, m.countKey(ws, agent, c, bucket), int64(n)).Err(); err != nil {
		return fmt.Errorf("agentvolume: refunding %d on %s: %w", n, c, err)
	}
	return nil
}
