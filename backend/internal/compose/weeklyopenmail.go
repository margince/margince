// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (w *weeklyGenerateWorker) openTasksMail(ctx context.Context, review weekly.Review, now time.Time, words mailcopy.Copy) (emailview.Content, error) {
	service := newAttentionService(w.pool, NewApprovalsService(InstallationDB(w.pool)), func() time.Time { return now })
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
	tasks, err := weekTasks(ctx, service)
	if err != nil {
		return emailview.Content{}, err
	}
	shown := 0
	var body emailview.Content
	for _, item := range tasks {
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
			break
		}
	}
	return body, nil
}

// weekTasks reads the reader's tasks due now, then those due after today, each
// once. A week still running holds tasks the Worklist files as upcoming.
func weekTasks(ctx context.Context, service *attention.Service) ([]crmcontracts.WorklistItem, error) {
	seen := map[string]bool{}
	var out []crmcontracts.WorklistItem
	for _, filter := range []crmcontracts.WorklistFilter{crmcontracts.WorklistFilterTasks, crmcontracts.WorklistFilterUpcoming} {
		cursor := ""
		for {
			day, err := service.Worklist(ctx, "mine", string(filter), ids.Nil, 100, cursor)
			if err != nil {
				return nil, err
			}
			for _, item := range day.Queue {
				if key := string(item.Source) + "/" + item.Id; !seen[key] {
					seen[key] = true
					out = append(out, item)
				}
			}
			if day.NextCursor == nil {
				break
			}
			cursor = *day.NextCursor
		}
	}
	return out, nil
}

func taskDueInReportedWeek(item crmcontracts.WorklistItem, end time.Time) bool {
	return item.Source == crmcontracts.WorklistItemSourceTask && deadline.Passed(item.DueAt, end)
}
