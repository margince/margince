// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"github.com/riverqueue/river"
)

// QueuedAs binds an insert to the queue its kind DECLARES, the way Govern
// binds a worker to its declared timeout.
//
// api/jobs.yaml names a queue for every kind, and that name is what the fleet
// surfaces publish and what jobqueues.go sizes a worker pool against. Before
// this it was also, for an insert built by hand, documentation the runtime
// never read: moving comms_send_email onto its own queue changed the
// declaration, `make check` went green, and every send carried on landing on
// `default`, because an InsertOpts naming no queue is River's own default and
// nothing compared the two.
//
// Supplying the value rather than checking it is what closes that for good —
// a fan-out child's queue has always come from the declaration this way, and
// there was never a reason the hand-built inserts should be the exception.
// The kind comes from the type rather than a string so a caller cannot name a
// kind the args do not answer to.
//
// A nil opts is an insert that asked for nothing else, and still gets its
// queue. An undeclared kind panics rather than falling through to River's
// default, which is the same answer compose's fan-out helpers give: a kind the
// contract does not name is a wiring mistake, and the default it would land on
// is a pool nobody sized for it.
func QueuedAs[T river.JobArgs](opts *river.InsertOpts) *river.InsertOpts {
	var args T
	spec, declared := SpecFor(args.Kind())
	if !declared {
		panic("jobs: enqueuing " + args.Kind() + ", which api/jobs.yaml does not declare")
	}
	queued := river.InsertOpts{}
	if opts != nil {
		queued = *opts
	}
	queued.Queue = spec.Queue
	return &queued
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
