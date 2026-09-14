// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

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
	got := replyConversationText(anchor, rows)
	if strings.Contains(got, secret) || strings.Count(got, "界") != 6*1200 || len([]rune(got)) > replyActivityMaxRunes {
		t.Fatalf("conversation bounds or withheld filtering failed: %d runes", len([]rune(got)))
	}
}
