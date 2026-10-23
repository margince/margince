// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// CustomerMeetingSQL shares the customer-link rule between capture and reporting.
// The activity alias must be a compile-time identifier.
func CustomerMeetingSQL(activityAlias string) string {
	arms := customerEvidenceArms(liveRecord(openDealPredicate, "d"), liveRecord(workingLeadPredicate, "ld"))
	clauses := make([]string, 0, len(arms))
	for _, arm := range arms {
		clauses = append(clauses, fmt.Sprintf(arm.sql, scopeUnbounded))
	}
	return "EXISTS(SELECT 1 FROM activity_link sales WHERE sales.activity_id=" + activityAlias + ".id AND (" + strings.Join(clauses, " OR ") + "))"
}

// appendCustomerMeetingClause narrows a list to customer meetings, counting
// only a customer link whose record this reader may see. A link to a record
// they cannot see would otherwise decide whether the meeting appears, which
// tells them the link is there.
func appendCustomerMeetingClause(ctx context.Context, in ListActivitiesInput, arg func(any) int, where []string) ([]string, error) {
	if !in.CustomerMeetingsOnly {
		return where, nil
	}
	customer, err := customerArms(ctx, liveRecord(openDealPredicate, "d"), liveRecord(workingLeadPredicate, "ld"), arg)
	if err != nil {
		return nil, err
	}
	visible, err := auth.LinkTargetVisibleClause(ctx, "sales", arg)
	if err != nil {
		return nil, err
	}
	if visible == "" {
		visible = scopeUnbounded
	}
	return append(where, `a.kind = 'meeting' AND EXISTS (SELECT 1 FROM activity_link sales WHERE sales.activity_id = a.id
	   AND `+customer+` AND (`+visible+`))`), nil
}
