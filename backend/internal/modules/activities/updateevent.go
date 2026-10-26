// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Events describe requested fields; audit images describe the persisted row.
// Body content stays out of events because it can be large and private. The
// precision is named whenever the time is, as in the audit images. A consumer
// then never reads a day's stored noon as an hour.
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
	if precision := eventPrecision(in, current); precision != "" {
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

// eventPrecision is the precision an update event names: the patch's whenever
// it names a time or a new precision, empty when it names neither.
func eventPrecision(in UpdateActivityInput, current crmcontracts.Activity) crmcontracts.PublicEventActivityChangedFieldsOccurredPrecision {
	precision := string(current.OccurredPrecision)
	if in.OccurredPrecision != nil {
		precision = *in.OccurredPrecision
	}
	if in.OccurredAt == nil && precision == string(current.OccurredPrecision) {
		return ""
	}
	return crmcontracts.PublicEventActivityChangedFieldsOccurredPrecision(precision)
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
