// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The duty endpoint retains its published vocabulary. Source assessments use the review endpoint.
func legacyNoticeStates() []string {
	return []string{
		string(NoticeOpen), string(NoticeAssigned), string(NoticeQueued), string(NoticeDeliveryFailed),
		string(NoticeCompleted), string(NoticeProvidedElsewhere), string(NoticeExemptWithReason), string(NoticeBlocked), string(NoticeNotRequired),
	}
}

func noticeDutyClause(arg func(any) int) string {
	var ended []string
	terminal := terminalNoticeStates()
	for _, state := range legacyNoticeStates() {
		if terminal[NoticeState(state)] {
			ended = append(ended, state)
		}
	}
	return storekit.SQLf("due_at IS NOT NULL AND rule = ANY($%d) AND state = ANY($%d) AND (state = ANY($%d) OR NOT EXISTS (SELECT 1 FROM contact_acquisition_evidence e WHERE e.id = privacy_notice_case.acquisition_id AND (", arg([]string{string(RuleArt13), string(RuleArt14)}), arg(legacyNoticeStates()), arg(ended)) + UnverifiedNoticeSourceSQL("privacy_notice_case", "e") + ")))"
}

func noticeDutyExistsTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := storekit.SQLf("id = $%d AND ", arg(id)) + noticeDutyClause(arg)
	var exists bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM privacy_notice_case WHERE "+where+")", args...).Scan(&exists)
	return exists, err
}

func (c NoticeCase) needsSourceReview() bool {
	if c.State == NoticeSourceReview {
		return true
	}
	if c.SourceKind != nil || c.Acquisition == nil || c.Acquisition.Kind != "unknown_legacy" {
		return false
	}
	return slices.Contains(SettleableNoticeStates(), string(c.State))
}

func sourceReviewConditionSQL(arg func(any) int) string {
	return storekit.SQLf("state = ANY($%d) AND EXISTS (SELECT 1 FROM contact_acquisition_evidence e WHERE e.id = privacy_notice_case.acquisition_id AND (", arg(SettleableNoticeStates())) + UnverifiedNoticeSourceSQL("privacy_notice_case", "e") + "))"
}

func reviewStateSQL(arg func(any) int) string {
	condition := sourceReviewConditionSQL(arg)
	return "CASE WHEN (" + condition + storekit.SQLf(") THEN $%d ELSE state END", arg(string(NoticeSourceReview)))
}

func reviewQueueTimeSQL(arg func(any) int) string {
	return "CASE WHEN (" + sourceReviewConditionSQL(arg) + ") THEN created_at ELSE coalesce(due_at, created_at) END"
}
