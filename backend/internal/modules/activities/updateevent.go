// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Events describe requested fields; audit images describe the persisted row.
// Body content stays out of events because it can be large and private. The
// precision is named only when it changed, since a new time alone implies one.
func activityUpdatedChangedFields(in UpdateActivityInput, current crmcontracts.Activity) crmcontracts.PublicEventActivityChangedFields {
	var fields crmcontracts.PublicEventActivityChangedFields
	if in.Subject != nil {
		fields.Subject = in.Subject
	}
	if in.Body != nil {
		bodyTouched := true
		fields.Body = &bodyTouched
	}
	if in.OccurredAt != nil {
		fields.OccurredAt = in.OccurredAt
	}
	if in.OccurredPrecision != nil && *in.OccurredPrecision != string(current.OccurredPrecision) {
		precision := crmcontracts.PublicEventActivityChangedFieldsOccurredPrecision(*in.OccurredPrecision)
		fields.OccurredPrecision = &precision
	}
	if in.DueAt != nil {
		fields.DueAt = in.DueAt
	}
	if in.RemindAt != nil {
		fields.RemindAt = in.RemindAt
	}
	if in.AssigneeID != nil {
		assignee := openapi_types.UUID(in.AssigneeID.UUID)
		fields.AssigneeId = &assignee
	}
	if in.IsDone != nil {
		fields.IsDone = in.IsDone
	}
	if in.MeetingStatus != nil {
		status := crmcontracts.PublicEventActivityChangedFieldsMeetingStatus(*in.MeetingStatus)
		fields.MeetingStatus = &status
	}
	return fields
}

// precisionBesideTime puts the precision in both audit images whenever the time
// is there. A moved day entry then still says its time is only a date.
func precisionBesideTime(before, after map[string]any, current, out crmcontracts.Activity) {
	if _, moved := after[auditFieldOccurredAt]; !moved {
		return
	}
	before[fieldOccurredPrecision] = string(current.OccurredPrecision)
	after[fieldOccurredPrecision] = string(out.OccurredPrecision)
}
