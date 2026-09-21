// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How this feed's composed reads reach the database as ONE unit of work.

import "context"

// Snapshots composes a multi-statement read inside a single database snapshot.
//
// AN INTERFACE RATHER THAN A POOL, so this package keeps taking readers and
// never a driver: the seam is satisfied by platform/database in compose, and by
// nothing at all in a unit test, which is what lets every lane test here drive
// the feed without standing a database up.
type Snapshots interface {
	// InSnapshot runs fn against one snapshot every read beneath it joins.
	InSnapshot(ctx context.Context, fn func(context.Context) error) error
	// Detached answers a ctx with no snapshot on it, for the one write a
	// composed read makes. The caller says why at its call site.
	Detached(ctx context.Context) context.Context
}

// inSnapshot runs fn inside this feed's snapshot.
//
// UNBOUND IS THE OLD BEHAVIOUR, exactly: every reader opens its own transaction
// and the day is composed from as many instants as it has lanes. That is what a
// unit test with no database gets, and it is why this is a nil check rather
// than a required dependency — the alternative is every test in this package
// carrying a seam it has no use for.
func (s *Service) inSnapshot(ctx context.Context, fn func(context.Context) error) error {
	if s.snapshots == nil {
		return fn(ctx)
	}
	return s.snapshots.InSnapshot(ctx, fn)
}

// detached answers the context a write makes its own transaction on.
func (s *Service) detached(ctx context.Context) context.Context {
	if s.snapshots == nil {
		return ctx
	}
	return s.snapshots.Detached(ctx)
}
