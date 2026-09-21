// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// store is where one Limiter's counts live. Allow, Record and Blocked are
// written against this interface rather than against either implementation, so
// moving a ceiling off this process is a change of backing store and not of
// anything a caller says.
//
// count and peek take the caller's clock reading because the in-process store
// is tested by advancing time rather than by sleeping against it. A shared
// store ignores it: the window there is Redis's own expiry, and a replica
// whose clock disagreed with its neighbours' would otherwise open a second
// window on a key that already had one.
type store interface {
	count(key string, span time.Duration, now time.Time) (int, error)
	peek(key string, span time.Duration, now time.Time) (int, error)
	forget(prefix string) error
}

// Registry is the backing store every Limiter built from it reads through.
//
// It exists so the store can be swapped ONCE, at assembly, and reach limiters
// already constructed: the composition root builds its handlers (and their
// ceilings) before it knows whether this deployment has a Redis to share, and
// re-plumbing every constructor to carry a client would be the interface
// change this is deliberately not.
type Registry struct {
	mu     sync.RWMutex
	shared store // nil until bound; each Limiter then uses its own local store
}

// NewRegistry builds a registry whose limiters each count in this process.
// That is the honest scope for a single replica and the default everywhere:
// a binary that never binds a shared store behaves exactly as it did before
// one existed.
func NewRegistry() *Registry { return &Registry{} }

// Shared builds a registry whose limiters all count in one Redis, so N
// replicas enforce ONE ceiling between them rather than N ceilings of the
// configured size.
func Shared(rdb *redis.Client) *Registry { return &Registry{shared: &sharedStore{rdb: rdb}} }

// RebindFrom moves this registry's limiters onto src's store — the boot-time
// injection point, modelled on agentvolume.Meter.RebindFrom for the same
// reason: it lets the composition root hold ceilings without naming a Redis
// client, which stays a dependency of cmd. Called at assembly, before any
// request is served, so it never races a count.
func (r *Registry) RebindFrom(src *Registry) {
	src.mu.RLock()
	s := src.shared
	src.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.shared = s
}

//nolint:ireturn // the interface is the answer: nil means no store is shared, which a concrete type cannot express.
func (r *Registry) sharedStore() store {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shared
}

// New builds a limiter on this registry. See the package-level New for what
// name and kind carry.
func (r *Registry) New(name string, kind Kind, limit int, span time.Duration) *Limiter {
	return r.NewWithClock(name, kind, limit, span, time.Now)
}

// NewWithClock takes the clock as a dependency so window expiry is a property
// tests assert by advancing time, not by sleeping against it. The clock
// governs the in-process store only — see store.
func (r *Registry) NewWithClock(name string, kind Kind, limit int, span time.Duration, now func() time.Time) *Limiter {
	if name == "" {
		// An unnamed limiter would share one Redis bucket with every other
		// unnamed one, silently spending a login budget on a webhook's
		// refusals. Refusing at construction makes that a boot failure rather
		// than a ceiling nobody can explain.
		panic("ratelimit: a limiter must name what it bounds")
	}
	return &Limiter{
		name:   name,
		kind:   kind,
		limit:  limit,
		window: span,
		now:    now,
		local:  newLocalStore(),
		reg:    r,
	}
}

// process is the registry every limiter built by the package-level New
// attaches to. One per process, because "these replicas share one ceiling" is
// a fact about the deployment and not about any one composition inside it.
var process = NewRegistry()

// ShareProcess moves every limiter in this process — those already built and
// those built after — onto one Redis-backed store. A role that serves any
// bounded edge calls it once at boot; a role that does not, or a deployment
// with no Redis to share, simply does not, and keeps the in-process ceilings
// that were the only ones before.
func ShareProcess(rdb *redis.Client) { process.RebindFrom(Shared(rdb)) }

// New builds a limiter on this process's registry.
//
// name says what is being bounded, in `area/subject` form. It is the shared
// store's key prefix, so it is the whole of what keeps two ceilings apart: two
// limiters that name the same thing count into the same window, on purpose or
// by accident, and the accident is silent.
//
// kind says what this limiter answers when the shared store cannot be reached.
func New(name string, kind Kind, limit int, span time.Duration) *Limiter {
	return process.New(name, kind, limit, span)
}

// NewWithClock is New with an injected clock. See Registry.NewWithClock.
func NewWithClock(name string, kind Kind, limit int, span time.Duration, now func() time.Time) *Limiter {
	return process.NewWithClock(name, kind, limit, span, now)
}
