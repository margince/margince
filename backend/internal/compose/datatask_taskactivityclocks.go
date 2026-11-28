// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// ActivityClocksTask repairs stored clocks under the current admission rule.
func ActivityClocksTask() datatask.Task {
	audit := datatask.AuditPolicy{Reason: clockRefoldAuditReason}
	return datatask.Task{
		ID: "core/1792846717_task_activity_clocks", Version: 2,
		Title: systemtext.Literal("datatask.title.core.1792846717_task_activity_clocks"), Requires: []string{"core/1795832146"},
		Steps: []datatask.Step{
			clockRefoldStep("contacts", storekit.ClockContacts, storekit.ClockNotFolded, audit),
			clockRefoldStep("companies", storekit.ClockCompanies, storekit.ClockNotFolded, audit),
			clockRefoldStep("deals", storekit.ClockDeals, storekit.ClockNotFolded, audit),
			clockRefoldStep("projects", storekit.ClockProjects, storekit.ClockNotFolded, audit),
		},
	}
}
