// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

const (
	replyContextTurns = 6
	// replyEvidenceRows is how many thread rows the language evidence orders
	// before it cuts. Newer outbound mail must not push an older inbound
	// message out of the window.
	replyEvidenceRows = 60
)

// replyThread is what a reply reads. The prompt shows a few turns. The
// language is decided from the wider set and the answered message.
type replyThread struct {
	prompt   []draftcore.Turn
	evidence []draftcore.Turn
	anchor   []draftcore.Turn
}

// addresseeEvidence is the addressee's own text, the answered message included.
func (t replyThread) addresseeEvidence(addressee string) []string {
	return draftcore.AddresseeEvidence(append(slices.Clone(t.anchor), t.evidence...), addressee)
}

// The selected message remains separate so a newer turn cannot become the reply target.
func (d replyDrafter) replyConversation(ctx context.Context, anchor crmcontracts.Activity, addressee string) (replyThread, error) {
	now := d.envelope.Now()
	thread := replyThread{anchor: draftcore.Conversation([]crmcontracts.Activity{anchor}, nil, now, 1)}
	if anchor.ThreadKey != nil && strings.TrimSpace(*anchor.ThreadKey) != "" {
		rows, err := d.threadRows(ctx, *anchor.ThreadKey, addressee)
		if err != nil {
			return replyThread{}, err
		}
		thread.prompt = draftcore.Conversation(rows, &anchor, now, replyContextTurns)
		thread.evidence = draftcore.Conversation(rows, &anchor, now, len(rows))
	}
	marked := append(slices.Clone(thread.anchor), thread.evidence...)
	d.markSenders(ctx, marked)
	thread.anchor, thread.evidence = marked[:len(thread.anchor)], marked[len(thread.anchor):]
	return thread, nil
}

// threadRows is the thread's newest page, newest first. When the thread is
// longer, the addressee's newest mail follows the page. So their own words
// still decide the language of a long thread.
func (d replyDrafter) threadRows(ctx context.Context, threadKey, addressee string) ([]crmcontracts.Activity, error) {
	limit, kind, now := replyEvidenceRows, string(crmcontracts.ActivityKindEmail), d.envelope.Now()
	// ConversationAsOf keeps a row dated after the drafting instant out of the
	// prompt and out of the language evidence.
	in := activities.ListActivitiesInput{
		ThreadKey: &threadKey, Kind: &kind, Limit: &limit, WrittenByHuman: true, ConversationAsOf: &now,
	}
	rows, page, err := d.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	sender, parseErr := ids.Parse(addressee)
	if !page.HasMore || parseErr != nil {
		return rows, nil
	}
	own := draftcore.LanguageEvidenceTurns
	in.Limit, in.SentBy = &own, &sender
	older, _, err := d.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	seen := make(map[crmcontracts.Id]bool, len(rows))
	for _, row := range rows {
		seen[row.Id] = true
	}
	for _, row := range older {
		if !seen[row.Id] {
			rows = append(rows, row)
		}
	}
	return rows, nil
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
			turn.At.Format("2006-01-02T15:04:05Z07:00"), direction, textcut.Runes(turn.Subject, 200), turn.Text))
	}
	return textcut.Runes(strings.Join(rendered, "\n\n"), replyActivityMaxRunes)
}
