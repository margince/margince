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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

const (
	replyContextTurns = 6
	// replyEvidenceRows is how many thread rows the language evidence orders
	// before it cuts. Newer outbound mail must not push an older inbound
	// message out of the window.
	replyEvidenceRows = 60
)

// replyThread is what a reply reads from its thread: the turns the prompt
// shows and the wider set the language is decided from.
type replyThread struct {
	prompt   []draftcore.Turn
	evidence []draftcore.Turn
}

// The selected message remains separate so a newer turn cannot become the reply target.
func (d replyDrafter) replyConversation(ctx context.Context, anchor crmcontracts.Activity) (replyThread, error) {
	if anchor.ThreadKey == nil || strings.TrimSpace(*anchor.ThreadKey) == "" {
		return replyThread{}, nil
	}
	limit, kind := replyEvidenceRows, string(crmcontracts.ActivityKindEmail)
	rows, _, err := d.store.ListActivities(ctx, activities.ListActivitiesInput{
		ThreadKey: anchor.ThreadKey, Kind: &kind, Limit: &limit, WrittenByHuman: true,
	})
	if err != nil {
		return replyThread{}, err
	}
	now, zone := d.envelope.Now(), d.envelope.Zone(ctx)
	thread := replyThread{
		prompt:   draftcore.Conversation(rows, &anchor, now, zone, replyContextTurns),
		evidence: draftcore.Conversation(rows, &anchor, now, zone, len(rows)),
	}
	d.markSenders(ctx, thread.evidence)
	return thread, nil
}

// markSenders records who sent each inbound evidence turn. A failed read
// leaves the turns unmarked, which ranks all inbound mail alike: the
// language decision degrades, the draft does not fail.
func (d replyDrafter) markSenders(ctx context.Context, turns []draftcore.Turn) {
	var inbound []ids.UUID
	for _, turn := range turns {
		if id, err := ids.Parse(turn.ID); err == nil && turn.Direction == string(crmcontracts.ActivityDirectionInbound) {
			inbound = append(inbound, id)
		}
	}
	senders, err := d.store.SenderContacts(ctx, inbound)
	if err != nil {
		d.logger().WarnContext(ctx, "reply senders unavailable; ranking inbound mail alike", "err", err)
		return
	}
	for i := range turns {
		if id, err := ids.Parse(turns[i].ID); err == nil {
			if sender, ok := senders[id]; ok {
				turns[i].SenderID = sender.String()
			}
		}
	}
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
			occurredday.When(occurredday.Moment{At: turn.At, DateOnly: turn.DateOnly}), direction, textcut.Runes(turn.Subject, 200), turn.Text))
	}
	return textcut.Runes(strings.Join(rendered, "\n\n"), replyActivityMaxRunes)
}
