// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import "github.com/riverqueue/river"

// SessionLocked marks work whose execution lease, rather than a River row,
// prevents concurrent runs. A dead owner's connection releases that lease.
type SessionLocked interface{ SessionLocked() }

func sessionInsertOpts(args river.JobArgs, opts *river.InsertOpts) *river.InsertOpts {
	if _, locked := args.(SessionLocked); !locked {
		return opts
	}
	owned := river.InsertOpts{}
	if opts != nil {
		owned = *opts
	}
	owned.UniqueOpts = river.UniqueOpts{}
	return &owned
}
