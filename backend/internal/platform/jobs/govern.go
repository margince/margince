// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

// DefaultMaxAttempts is the ladder River gives a job whose insert names none:
// 25 attempts on attempt-to-the-fourth backoff, which reaches days.
//
// It is spelled here, in the package that owns this repo's River integration,
// because the fitness function that refuses an uncapped insert has to know the
// number it is refusing and may not import River to ask. Mirrored rather than
// copied: a River upgrade that moves the default moves this with it.
const DefaultMaxAttempts = river.MaxAttemptsDefault

// WorkOnly is all a hand-written worker exposes. River asks a worker four
// questions; three of them — Timeout, NextRetry, Middleware — are the
// contract's to answer, so a worker that declared its own would be answering
// for the file. Narrowing the interface is what makes that impossible rather
// than merely discouraged: an embedded override is shadowed by the outer
// type, so a marker interface or a linter rule would both have missed it.
type WorkOnly[T river.JobArgs] interface {
	Work(context.Context, *river.Job[T]) error
}

// governedWorker is what River actually registers. The wrapped worker is
// reached only through Work, so any option method it happens to carry is
// unreachable.
type governedWorker[T river.JobArgs] struct {
	river.WorkerDefaults[T]
	work    WorkOnly[T]
	timeout time.Duration
}

func (w governedWorker[T]) Timeout(*river.Job[T]) time.Duration { return w.timeout }

func (w governedWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	return w.work.Work(ctx, job)
}

// Govern binds a worker to its declaration. supplied is used only by a
// TimeoutPolicy that declares {operator: …}; every other policy ignores it.
func Govern[T river.JobArgs](w WorkOnly[T], s Spec, supplied time.Duration) river.Worker[T] {
	return governedWorker[T]{work: w, timeout: s.Timeout.Duration(supplied)}
}
