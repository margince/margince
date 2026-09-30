// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package ratelimit is a small fixed-window limiter for the callers that must
// bound how often something happens per key.
//
// Two shapes of caller, and the API serves both. Allow counts an ATTEMPT and
// decides in one step — the unauthenticated auth endpoints take this one, since
// login brute-force is expensive to serve (Argon2id ≈ 19 MiB per attempt) and
// bootstrap mints whole tenants. Blocked and Record split the two halves for a
// caller that meters an OUTCOME instead: Blocked peeks without spending a slot,
// and Record spends one only once the metered thing actually happened. Outbound
// send pacing (comms.MailboxRatePolicy) takes that pair, because merely asking
// whether a mailbox may send must not consume its quota.
//
// Keys are bounded: one longer than maxKeyLen is not metered at all, on any of
// the three entry points, so a caller may key on a value a client chose without
// first bounding it itself.
//
// # Where the counts live
//
// In this process by default, which is one ceiling only while there is one
// replica: two replicas each counting their own view admit twice the
// configured rate, and an attacker who can reach both gets the ceiling
// multiplied rather than enforced. A role that binds a shared store
// (ShareProcess) moves every one of its limiters into Redis, where N replicas
// read and write ONE window per key. Nothing a caller says changes either way
// — see Registry for why the swap happens at assembly rather than at each
// construction.
//
// # What a limiter says when it cannot count
//
// A shared store can be unreachable, and then the limiter has no ceiling to
// enforce. Which way it fails is not a detail the caller may leave open, so
// every limiter declares a Kind when it is built — see Kind for how to choose
// between them.
package ratelimit

import (
	"log/slog"
	"sync/atomic"
	"time"
)

// Kind is what a limiter answers when it cannot reach its shared store.
//
// The question at each site is not which category the endpoint belongs to. It
// is what one window of UNMETERED traffic costs, weighed against what one
// window of refusing everybody costs — and a limiter is built with the answer
// because the moment it is asked is the moment it can no longer work it out.
type Kind int

const (
	// FailClosed refuses. It is the posture for a bound whose unmetered window
	// buys something that cannot be taken back afterwards: a guessed password,
	// a minted link, a created tenant, a provider account throttled for the
	// day. It is also the posture for every bound an attacker would like to
	// be rid of, because the alternative hands them the way to be rid of it —
	// make Redis unavailable and every ceiling opens at once. Refusing looks
	// wrong at a glance, since a dependency outage then holds real callers out
	// of login, and it is still the one to keep.
	FailClosed Kind = iota
	// FailOpen admits. It is the posture for a bound whose unmetered window
	// costs only traffic — throughput protection for callers who are already
	// past authentication, where nothing is spent that the next window cannot
	// undo, and where refusing would take a working surface down for everybody
	// holding a valid credential.
	FailOpen
)

// maxKeyLen bounds what may become a counter key. Every caller keys on
// something a remote client chose — a path segment, a slug, a token, an email
// — and Go admits a request line near 1 MB, so an unbounded key turns a
// protective edge into a memory-exhaustion path: what the store takes stays
// until its window expires, which never happens for a key the flood keeps
// refreshing. Bounding it here rather than at each call site is what stops the
// next caller reintroducing it.
//
// The bound is generous rather than tight, because an unmetered key is a hole
// in whatever the caller was rate-limiting and the longest LEGITIMATE key here
// is not short: identity meters login failures on `email|IP`, and RFC 5321
// permits a 254-character address, which an IPv6 literal takes past 300. A tight
// bound would silently stop metering exactly the accounts with long addresses.
// At this size no honest key is refused, and the memory ceiling is still three
// orders of magnitude below what an unbounded key allows.
const maxKeyLen = 512

// meterable reports whether key is one this limiter will account for.
//
// An over-long key is refused, never truncated: truncation merges distinct
// callers into one bucket, so a single long shared prefix would spend everyone
// else's budget. Refusing means such a key is simply not metered — Allow admits
// it, Blocked never reports it, Record counts nothing. That direction is
// deliberate. A composite key can legitimately run long (an RFC 5321 address
// paired with an IPv6 literal already can), and denying on length would lock a
// real caller out of a real account for the size of a value they did not
// choose. So a caller that keys on a value a client chose must also meter a
// bounded one — the client IP — because that second budget is what brakes a
// flood whether or not the first key was meterable.
func meterable(key string) bool { return len(key) <= maxKeyLen }

// Limiter counts events per key in fixed windows. The zero value is not
// usable; construct with New.
type Limiter struct {
	name   string
	kind   Kind
	limit  int
	window time.Duration
	now    func() time.Time

	// local counts when no shared store is bound. Per Limiter rather than per
	// Registry, so two limiters carrying one name do not share buckets in a
	// test binary that builds that name once per case.
	local store
	reg   *Registry

	// mutedUntil throttles the outage notice below. An unreachable store fails
	// on every request, and a line per request is a second outage on the log
	// pipeline at exactly the moment an operator needs it readable.
	mutedUntil atomic.Int64
}

// Allow records one attempt for key and reports whether it is within the
// limit. Counting before deciding means an attacker cannot probe the
// limit boundary for free.
func (l *Limiter) Allow(key string) bool {
	if !meterable(key) {
		return true
	}
	n, err := l.store().count(l.scoped(key), l.window, l.now())
	if err != nil {
		return l.admitsBlind(err, "allow")
	}
	return n <= l.limit
}

// Record counts one event for key without deciding. Paired with Blocked
// for limiters that count OUTCOMES (failed logins) rather than attempts —
// counting every attempt would let an attacker's noise throttle a
// legitimate caller's successes.
func (l *Limiter) Record(key string) {
	if !meterable(key) {
		return
	}
	if _, err := l.store().count(l.scoped(key), l.window, l.now()); err != nil {
		// Nothing to decide here, so the posture cannot be applied: an
		// uncounted outcome is simply lost, and the Blocked that reads it
		// later is the call that answers for the gap.
		l.noteOutage(err, "record")
	}
}

// Blocked reports whether key has already reached the limit in its
// current window, without counting the probe.
func (l *Limiter) Blocked(key string) bool {
	if !meterable(key) {
		return false
	}
	n, err := l.store().peek(l.scoped(key), l.window, l.now())
	if err != nil {
		return !l.admitsBlind(err, "blocked")
	}
	return n >= l.limit
}

// Reset forgets every bucket, admitting the next attempt on every key.
//
// It exists for the non-production data reset: a lockout that outlives the
// data it protected reads as a broken installation, and the operator who just
// wiped the install is the same human the limiter would be holding out.
func (l *Limiter) Reset() {
	if err := l.store().forget(l.scoped("")); err != nil {
		l.noteOutage(err, "reset")
	}
}

// store is the shared one when a role has bound it, and this limiter's own
// otherwise. Read per call rather than captured at construction, because the
// binding happens after the composition root has built its handlers.
//
//nolint:ireturn // choosing between two backing stores IS this method; a concrete return type would be one of the two answers it exists to pick between.
func (l *Limiter) store() store {
	if s := l.reg.sharedStore(); s != nil {
		return s
	}
	return l.local
}

// scoped is the key as the store sees it. The name prefix is what keeps two
// ceilings apart in a store they share, and it is also what Reset deletes by.
func (l *Limiter) scoped(key string) string { return "ratelimit:" + l.name + ":" + key }

// admitsBlind reports what this limiter answers when it could not count, and
// says so in the log once per outageNotice so the operator sees an unenforced
// ceiling rather than only its effect.
func (l *Limiter) admitsBlind(err error, op string) bool {
	l.noteOutage(err, op)
	return l.kind == FailOpen
}

func (l *Limiter) noteOutage(err error, op string) {
	now := l.now()
	muted := l.mutedUntil.Load()
	if now.UnixNano() < muted || !l.mutedUntil.CompareAndSwap(muted, now.Add(outageNotice).UnixNano()) {
		return
	}
	slog.Default().Error("a rate limiter could not reach its shared store; the ceiling is not being enforced",
		"limiter", l.name, "op", op, "admitting", l.kind == FailOpen, "err", err)
}

// outageNotice is how often one limiter repeats its outage line. Long enough
// that a sustained outage is a handful of lines a minute across every ceiling,
// short enough that the line is still there when somebody looks.
const outageNotice = 10 * time.Second
