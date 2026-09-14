// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
)

const replyContextTurns = 6

// The selected message remains separate so a newer turn cannot become the reply target.
func (d replyDrafter) replyConversation(ctx context.Context, anchor crmcontracts.Activity) (string, error) {
	if anchor.ThreadKey == nil || strings.TrimSpace(*anchor.ThreadKey) == "" {
		return "", nil
	}
	// Reserve a slot for the selected anchor, which may be in the page.
	limit, kind := replyContextTurns+1, string(crmcontracts.ActivityKindEmail)
	rows, _, err := d.store.ListActivities(ctx, activities.ListActivitiesInput{
		ThreadKey: anchor.ThreadKey, Kind: &kind, Limit: &limit,
	})
	if err != nil {
		return "", err
	}
	return replyConversationText(anchor, rows), nil
}

func replyConversationText(anchor crmcontracts.Activity, rows []crmcontracts.Activity) string {
	var turns []string
	for _, row := range rows {
		if row.Id == anchor.Id || row.Kind != crmcontracts.ActivityKindEmail ||
			row.ContentState != nil && *row.ContentState == crmcontracts.ActivityContentStateWithheld {
			continue
		}
		direction := "unknown"
		if row.Direction != nil {
			direction = string(*row.Direction)
		}
		relation := "at or before the selected message"
		if row.OccurredAt.After(anchor.OccurredAt) {
			relation = "later than the selected message"
		}
		turns = append(turns, fmt.Sprintf("%s | %s | %s | %s\n%s", relation, row.OccurredAt.Format("2006-01-02T15:04:05Z07:00"),
			direction, boundedRunes(stringValue(row.Subject), 200), boundedRunes(stringValue(row.Body), 1200)))
		if len(turns) == replyContextTurns {
			break
		}
	}
	return boundedRunes(strings.Join(turns, "\n\n"), replyActivityMaxRunes)
}
