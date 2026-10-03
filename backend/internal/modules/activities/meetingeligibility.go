// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// CustomerMeetingSQL shares the customer-link rule between capture and reporting.
// The activity alias must be a compile-time identifier.
func CustomerMeetingSQL(activityAlias string) string {
	return "EXISTS(SELECT 1 FROM activity_link al WHERE al.activity_id=" + activityAlias + ".id AND " + customerLinkSQL + ")"
}

// customerLinkSQL is what makes a link under alias al a customer link.
const customerLinkSQL = "(al.contact_id IS NOT NULL OR al.lead_id IS NOT NULL OR al.company_id IS NOT NULL)"

// appendCustomerMeetingClause narrows a list to customer meetings, counting
// only a customer link whose record this reader may see. A link to a record
// they cannot see would otherwise decide whether the meeting appears, which
// tells them the link is there.
func appendCustomerMeetingClause(ctx context.Context, in ListActivitiesInput, arg func(any) int, where []string) ([]string, error) {
	if !in.CustomerMeetingsOnly {
		return where, nil
	}
	visible, err := auth.LinkTargetVisibleClause(ctx, "al", arg)
	if err != nil {
		return nil, err
	}
	if visible == "" {
		visible = scopeUnbounded
	}
	return append(where, `EXISTS (SELECT 1 FROM activity_link al WHERE al.activity_id = a.id
	   AND `+customerLinkSQL+` AND (`+visible+`))`), nil
}
