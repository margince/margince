// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"github.com/riverqueue/river"
)

// QueuedAs takes the declared queue so an insert cannot choose a conflicting pool.
func QueuedAs[T river.JobArgs](opts *river.InsertOpts) *river.InsertOpts {
	var args T
	return QueuedAsArgs(args, opts)
}

// QueuedAsArgs applies the same queue and ownership policy to fan-out args.
func QueuedAsArgs(args river.JobArgs, opts *river.InsertOpts) *river.InsertOpts {
	return SessionInsertOpts(args, QueuedAsKind(args.Kind(), opts))
}

// QueuedAsKind is QueuedAs for a caller that knows the kind only as a string —
// a fan-out helper handed a child kind at runtime, where no Go type is
// available to name. The type parameter is the better door and every caller
// that can use it does; this one exists so the dynamic cases still go through
// the same single writer of a queue.
func QueuedAsKind(kind string, opts *river.InsertOpts) *river.InsertOpts {
	spec, declared := SpecFor(kind)
	if !declared {
		panic("jobs: enqueuing " + kind + ", which api/jobs.yaml does not declare")
	}
	queued := river.InsertOpts{}
	if opts != nil {
		queued = *opts
	}
	queued.Queue = spec.Queue
	return &queued
}
