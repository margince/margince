// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package pipelinetrace

// What the signal extractor's rule says about ONE conversation.
//
// Declared here and implemented by compose, which owns the rule, for the reason
// this package's header gives: it does not re-derive any module's rule. The
// extractor's arms are named where the queue composes them
// (compose/signalextractrule.go) and asked here through this interface, so the
// answer a member reads and the decision the pass makes are the same sentence.
//
// The direction is what keeps it honest: compose imports this package, so the
// rule's owner supplies the reader rather than this package reaching for it.

import "context"

// ThreadReading is the extractor's answer for one thread. Every field is one of
// the arms the offer is composed from, plus what has already happened to it.
type ThreadReading struct {
	// Known is false when the thread has no conversation at all in the
	// extractor's sense — every message archived, or none captured by a
	// connector. Nothing below is meaningful then.
	Known bool

	ReachesOneAccount bool
	IsOneBodyOfWork   bool
	IsFullyOpen       bool
	HasANamedReader   bool
	HasSettled        bool
	IsParked          bool
	HasMoved          bool

	// Scanned is true once a reading has recorded a cursor for this thread.
	Scanned bool
	// Cited is true when a signal on this conversation cites this message.
	Cited bool
}

// ThreadReader answers for one thread. A deployment that composed no signal
// extractor supplies none, and the rung says so rather than guessing.
type ThreadReader interface {
	ReadThread(ctx context.Context, threadKey string, activityID string) (ThreadReading, error)
}
