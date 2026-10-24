// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// taskActivityClocksDataTask refolds every stored clock the rule of 1792846717
// would fold otherwise. That is a clock a task set, and a project clock a
// called-off or future meeting set.
func taskActivityClocksDataTask() datatask.Task {
	audit := datatask.AuditPolicy{Reason: "the last-activity triggers move the same derived clock without an audit"}
	return datatask.Task{
		ID: "core/1792846717_task_activity_clocks", Version: 1,
		Title: "Recompute last-activity dates without tasks", Requires: []string{"core/1792846717"},
		Steps: []datatask.Step{
			clockRefoldStep("contacts", storekit.ClockContacts, storekit.ClockNotFolded, audit),
			clockRefoldStep("companies", storekit.ClockCompanies, storekit.ClockNotFolded, audit),
			clockRefoldStep("deals", storekit.ClockDeals, storekit.ClockNotFolded, audit),
			clockRefoldStep("projects", storekit.ClockProjects, storekit.ClockNotFolded, audit),
		},
	}
}
