// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"maps"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// activitySortFieldsIn is activitySortFields with the due order ending a chosen
// date in zone, so the sort agrees with the overdue test of the same read.
// Empty is the installation's zone.
func activitySortFieldsIn(zone string) map[string]storekit.SortField {
	if zone == "" {
		return activitySortFields
	}
	fields := maps.Clone(activitySortFields)
	fields["due_at"] = storekit.SortField{
		Kind: storekit.KindTimestamp, CursorField: "task_due_v3",
		Expr: func(_ context.Context, arg func(any) int) (string, error) {
			return storekit.TaskDueOrderInSQL("a", storekit.DayZoneSQL(zone, arg)), nil
		},
	}
	return fields
}
