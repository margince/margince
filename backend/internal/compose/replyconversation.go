// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

const replyContextTurns = 6

// The selected message remains separate so a newer turn cannot become the reply target.
func (d replyDrafter) replyConversation(ctx context.Context, anchor crmcontracts.Activity) ([]draftcore.Turn, error) {
	if anchor.ThreadKey == nil || strings.TrimSpace(*anchor.ThreadKey) == "" {
		return nil, nil
	}
	// Reserve a slot for the selected anchor, which may be in the page.
	limit, kind := replyContextTurns+1, string(crmcontracts.ActivityKindEmail)
	rows, _, err := d.store.ListActivities(ctx, activities.ListActivitiesInput{
		ThreadKey: anchor.ThreadKey, Kind: &kind, Limit: &limit, WrittenByHuman: true,
	})
	if err != nil {
		return nil, err
	}
	return draftcore.Conversation(rows, &anchor, d.envelope.Now(), replyContextTurns), nil
}

// replyConversationText renders the turns the way the reply prompt describes
// them. Each shows where it stands against the selected message, its date,
// direction and subject, then its text.
func replyConversationText(turns []draftcore.Turn) string {
	rendered := make([]string, 0, len(turns))
	for _, turn := range turns {
		direction := turn.Direction
		if direction == "" {
			direction = "unknown" //nolint:goconst // the prompt's word for a turn without direction, unrelated to preview lifecycles
		}
		relation := "at or before the selected message"
		if turn.AfterAnchor {
			relation = "later than the selected message"
		}
		rendered = append(rendered, fmt.Sprintf("%s | %s | %s | %s\n%s", relation,
			turn.At.Format("2006-01-02T15:04:05Z07:00"), direction, textcut.Runes(turn.Subject, 200), turn.Text))
	}
	return textcut.Runes(strings.Join(rendered, "\n\n"), replyActivityMaxRunes)
}
