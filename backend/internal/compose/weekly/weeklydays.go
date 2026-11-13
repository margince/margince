// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// daysInWeek is how many local days the review charts, Monday first.
const daysInWeek = 7

// countWeekDays counts the rep's own work on each local day of the week.
// Authorship is the rule that decides who may edit an activity, so a received
// email or a colleague's call never counts here.
func countWeekDays(ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time) ([]crmcontracts.WeeklyDayActivity, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	// Each bound is a local midnight, so a DST day is 23 or 25 hours long.
	bounds := make([]time.Time, daysInWeek+1)
	for i := range bounds {
		bounds[i] = start.AddDate(0, 0, i)
	}
	pos := func(v any) string { return "$" + strconv.Itoa(arg(v)) }
	startAt, endAt, boundsAt, seat := pos(start), pos(end), pos(bounds), arg(userID)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	authored := auth.ActivityAuthorSQL("a", seat, arg)
	credited := storekit.MeetingCreditSQL("a", "$"+strconv.Itoa(seat))
	rows, err := tx.Query(ctx, `
		SELECT width_bucket(`+occurredday.StartSQL("a")+`, `+boundsAt+`::timestamptz[]) AS day,
		       count(*) FILTER (WHERE a.kind = 'email'),
		       count(*) FILTER (WHERE a.kind = 'call'),
		       count(*) FILTER (WHERE a.kind = 'meeting')
		  FROM activity a
		 WHERE a.archived_at IS NULL
		   AND `+occurredday.WithinSQL("a", startAt, endAt)+`
		   AND (`+orUnbounded(scope)+`)
		   AND ((a.kind = 'email' AND `+relstrength.WeWroteSQL("a")+` AND `+authored+`)
		     OR (a.kind = 'call' AND `+authored+`)
		     OR (a.kind = 'meeting' AND (a.meeting_status IS NULL OR a.meeting_status = 'held') AND `+credited+`
		         AND NOT `+meetingoutcome.RecordsMeetingSQL("a")+`))
		 GROUP BY 1`, args...)
	if err != nil {
		return nil, fmt.Errorf("weekly: counting the week's days: %w", err)
	}
	defer rows.Close()
	days := make([]crmcontracts.WeeklyDayActivity, daysInWeek)
	for i := range days {
		local := bounds[i]
		days[i].Day = openapi_types.Date{Time: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)}
	}
	for rows.Next() {
		var bucket, emails, calls, meetings int
		if err := rows.Scan(&bucket, &emails, &calls, &meetings); err != nil {
			return nil, fmt.Errorf("weekly: reading the week's days: %w", err)
		}
		// width_bucket answers 1 to 7 for an instant inside the bounds.
		if bucket < 1 || bucket > daysInWeek {
			continue
		}
		day := &days[bucket-1]
		day.EmailsSent, day.CallsLogged, day.MeetingsHeld = emails, calls, meetings
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("weekly: reading the week's days: %w", err)
	}
	return days, nil
}
