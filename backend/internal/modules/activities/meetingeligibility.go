// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// CustomerMeetingSQL shares the customer-link rule between capture and reporting.
// The activity alias must be a compile-time identifier.
func CustomerMeetingSQL(activityAlias string) string {
	return "EXISTS(SELECT 1 FROM activity_link al WHERE al.activity_id=" + activityAlias + ".id AND (al.contact_id IS NOT NULL OR al.lead_id IS NOT NULL OR al.company_id IS NOT NULL))"
}
