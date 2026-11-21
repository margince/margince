// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
)

// TaskDueInReportedWeek says whether an open task falls due by the end of the
// reported week. A task with only a date is judged by the company's calendar.
func TaskDueInReportedWeek(item crmcontracts.WorklistItem, end time.Time, zone string) (bool, error) {
	if item.Source != crmcontracts.WorklistItemSourceTask {
		return false, nil
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return false, err
	}
	wall := ""
	if item.DueIntent != nil {
		wall = item.DueIntent.WallClock
	}
	return deadline.TaskPassed(item.DueAt, item.DueHasTime != nil && *item.DueHasTime, wall, end, loc), nil
}
