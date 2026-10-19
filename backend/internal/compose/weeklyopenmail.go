// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (w *weeklyGenerateWorker) openTasksMail(ctx context.Context, review weekly.Review, now time.Time, words mailcopy.Copy) (emailview.Content, error) {
	service := newAttentionService(w.pool, approvals.NewService(InstallationDB(w.pool)), func() time.Time { return now })
	hours, _, err := w.users.MyWorkingHours(ctx)
	if err != nil {
		return emailview.Content{}, err
	}
	zone, err := hours.Location()
	if err != nil {
		return emailview.Content{}, err
	}
	reportZone := "UTC"
	if review.NumericSummary != nil && review.NumericSummary.Timezone != "" {
		reportZone = review.NumericSummary.Timezone
	}
	_, weekEnd, err := weekly.WeekWindow(review.LocalWeekStart, reportZone)
	if err != nil {
		return emailview.Content{}, err
	}
	cursor, shown := "", 0
	var body emailview.Content
	for {
		day, err := service.Worklist(ctx, "mine", "tasks", ids.Nil, 100, cursor)
		if err != nil {
			return emailview.Content{}, err
		}
		for _, item := range day.Queue {
			if !taskDueInReportedWeek(item, weekEnd) {
				continue
			}
			line := mailWorkItem(item, words, zone)
			if line == "" {
				continue
			}
			if shown == 0 {
				body.Heading(words.MailOpenNow)
			}
			body.Paragraph(line)
			writeMailItemLink(&body, item, w.mail.PublicBaseURL, words)
			shown++
			if shown == 3 {
				return body, nil
			}
		}
		if day.NextCursor == nil {
			break
		}
		cursor = *day.NextCursor
	}
	return body, nil
}

func taskDueInReportedWeek(item crmcontracts.WorklistItem, end time.Time) bool {
	return item.Source == crmcontracts.WorklistItemSourceTask && deadline.Passed(item.DueAt, end)
}
