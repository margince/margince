// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import "github.com/riverqueue/river"

// FleetWide is implemented by the args of a job that answers for the whole
// installation. That is a scheduled pass, or a dispatcher that enqueues
// children for it. Its args carry no workspace.
//
// The marker method is empty. It is a declaration, not behaviour, and the G1
// gate reads it.
type FleetWide interface {
	river.JobArgs
	FleetWide()
}
