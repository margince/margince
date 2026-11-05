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
)

const (
	// replyEvidenceRows is how many thread rows the language evidence orders
	// before it cuts. Newer outbound mail must not push an older inbound
	// message out of the window.
	replyEvidenceRows = 60
)

// The selected message and wider language evidence have separate roles.
type replyThread struct {
	evidence []draftcore.Turn
	anchor   []draftcore.Turn
}

// addresseeEvidence is the addressee's own text, the answered message included.
func (t replyThread) addresseeEvidence(addressee string) []string {
	return draftcore.AddresseeEvidence(append(slices.Clone(t.anchor), t.evidence...), addressee)
}

// The selected message remains separate so a newer turn cannot become the reply target.
func (d replyDrafter) replyConversation(ctx context.Context, anchor crmcontracts.Activity, delivery activities.ReplyAddressee) (replyThread, error) {
	now, zone := d.envelope.Now(), d.envelope.Zone(ctx)
	thread := replyThread{anchor: draftcore.Conversation([]crmcontracts.Activity{anchor}, nil, now, zone, 1)}
	if anchor.ThreadKey != nil && strings.TrimSpace(*anchor.ThreadKey) != "" {
		rows, err := d.threadRows(ctx, anchor, delivery)
		if err != nil {
			return replyThread{}, err
		}
		thread.evidence = draftcore.Conversation(rows, &anchor, now, zone, len(rows))
	}
	marked := append(slices.Clone(thread.anchor), thread.evidence...)
	d.markSenders(ctx, marked)
	thread.anchor, thread.evidence = marked[:len(thread.anchor)], marked[len(thread.anchor):]
	return thread, nil
}

// threadRows is the thread's newest page, newest first. When the thread is
// longer, the addressee's newest mail follows the page. So their own words
// still decide the language of a long thread.
func (d replyDrafter) threadRows(ctx context.Context, anchor crmcontracts.Activity, delivery activities.ReplyAddressee) ([]crmcontracts.Activity, error) {
	addressee := delivery.ContactID
	limit, kind, now := replyEvidenceRows, string(crmcontracts.ActivityKindEmail), d.envelope.Now()
	anchorID := ids.UUID(anchor.Id)
	// A thread key is read from headers its sender writes, so it can name another
	// correspondence; ConversationOf keeps only rows shared with the anchor's correspondent.
	in := activities.ListActivitiesInput{
		ThreadKey: anchor.ThreadKey, ConversationOf: &anchorID, ConversationDeliveredTo: &delivery.Address,
		Kind: &kind, Limit: &limit, WrittenByHuman: true, ConversationAsOf: &now,
	}
	rows, page, err := d.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	if !page.HasMore || addressee == "" {
		return rows, nil
	}
	sender, err := ids.Parse(addressee)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the reply's addressee id: %w", err)
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
