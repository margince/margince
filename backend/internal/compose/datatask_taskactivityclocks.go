// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// taskActivityClocksDataTask refolds the stored clocks a task set before
// 1792846717 stopped counting tasks as activity.
func taskActivityClocksDataTask() datatask.Task {
	audit := datatask.AuditPolicy{Reason: "the last-activity triggers move the same derived clock without an audit"}
	selection := clockSelection{count: storekit.CountTaskClocks, next: storekit.TaskClocks}
	return datatask.Task{
		ID: "core/1792846717_task_activity_clocks", Version: 1,
		Title: "Take tasks out of last-activity dates", Requires: []string{"core/1792846717"},
		Steps: []datatask.Step{
			clockRefoldStep("contacts", storekit.ClockContacts, audit, selection),
			clockRefoldStep("companies", storekit.ClockCompanies, audit, selection),
			clockRefoldStep("deals", storekit.ClockDeals, audit, selection),
			clockRefoldStep("projects", storekit.ClockProjects, audit, selection),
		},
	}
}
