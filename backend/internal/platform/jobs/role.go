// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import "github.com/riverqueue/river"

// FleetWide is implemented by the args of a job that answers for the whole
// installation: a scheduled pass, or a dispatcher that enqueues children for
// it. Nothing may read a workspace off its args, because it carries none.
//
// The marker method is empty on purpose — it is a declaration, not
// behaviour, and it is what the G1 gate reads.
type FleetWide interface {
	river.JobArgs
	FleetWide()
}
