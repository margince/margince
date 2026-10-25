// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Events describe requested fields; audit images describe the persisted row.
// Body content stays out of events because it can be large and private.
func activityUpdatedChangedFields(in UpdateActivityInput) crmcontracts.PublicEventActivityChangedFields {
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
	if in.OccurredPrecision != nil {
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
