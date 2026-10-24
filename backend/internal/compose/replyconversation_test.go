// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReplyConversationOmitsWithheldMessagesAndBoundsTheEvidence(t *testing.T) {
	anchor := crmcontracts.Activity{Id: crmcontracts.Id(ids.NewV7()), Kind: crmcontracts.ActivityKindEmail}
	withheld := crmcontracts.ActivityContentStateWithheld
	secret, longBody := "private text", strings.Repeat("界", 2000)
	rows := []crmcontracts.Activity{anchor, {Id: crmcontracts.Id(ids.NewV7()), Kind: crmcontracts.ActivityKindEmail, ContentState: &withheld, Body: &secret}}
	for range 10 {
		rows = append(rows, crmcontracts.Activity{Id: crmcontracts.Id(ids.NewV7()), Kind: crmcontracts.ActivityKindEmail, Body: &longBody})
	}
	got := replyConversationText(draftcore.Conversation(rows, &anchor, time.Now(), replyContextTurns))
	if strings.Contains(got, secret) || strings.Count(got, "界") != 6*draftcore.TurnTextRunes || len([]rune(got)) > replyActivityMaxRunes {
		t.Fatalf("conversation bounds or withheld filtering failed: %d runes", len([]rune(got)))
	}
}

// The reply drafter and the contact drafter read one conversation: the same
// rows give the same turn text, whichever button the rep pressed.
func TestTheReplyAndContactDraftersReadTheSameTurns(t *testing.T) {
	out := crmcontracts.ActivityDirectionOutbound
	body := strings.Repeat("We asked about 40 seats. ", 60)
	mail := func(hoursAgo int) crmcontracts.Activity {
		subject := "Pricing"
		return crmcontracts.Activity{
			Id: crmcontracts.Id(ids.NewV7()), Kind: crmcontracts.ActivityKindEmail, Direction: &out,
			Subject: &subject, Body: &body, OccurredAt: time.Now().Add(-time.Duration(hoursAgo) * time.Hour),
		}
	}
	rows := []crmcontracts.Activity{mail(1), mail(2)}
	now := time.Now()

	reply := replyConversationText(draftcore.Conversation(rows, nil, now, replyContextTurns))
	for i, recent := range contactdraft.FoldRecent(rows, now) {
		if recent.Said == "" || !strings.Contains(reply, recent.Said) {
			t.Errorf("turn %d: the contact drafter reads %q, which the reply drafter does not", i, recent.Said)
		}
	}
}
