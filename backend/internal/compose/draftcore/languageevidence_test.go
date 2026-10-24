// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestLanguageEvidenceReadsTheRecipientsOwnMailFirst(t *testing.T) {
	t.Parallel()
	in, out := crmcontracts.ActivityDirectionInbound, crmcontracts.ActivityDirectionOutbound
	rows := []crmcontracts.Activity{
		turnRow(crmcontracts.ActivityKindEmail, out, 1, "our newest"),
		turnRow(crmcontracts.ActivityKindEmail, in, 5, "their newer"),
		turnRow(crmcontracts.ActivityKindEmail, in, 9, "their older"),
		turnRow(crmcontracts.ActivityKindEmail, out, 3, "our middle"),
	}
	got := draftcore.LanguageEvidenceOf(rows, turnsNow)
	want := []string{"their newer", "their older", "our newest", "our middle"}
	if len(got) != len(want) {
		t.Fatalf("got %d strings %q, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if !strings.HasSuffix(got[i], w) || !strings.HasPrefix(got[i], "Pricing\n") {
			t.Errorf("evidence[%d] = %q, want subject then %q", i, got[i], w)
		}
	}
}

func TestLanguageEvidenceDropsWithheldAndEmptyRowsAndStaysBounded(t *testing.T) {
	t.Parallel()
	in := crmcontracts.ActivityDirectionInbound
	withheld := turnRow(crmcontracts.ActivityKindEmail, in, 1, "secret words")
	state := crmcontracts.ActivityContentStateWithheld
	withheld.ContentState = &state
	empty := turnRow(crmcontracts.ActivityKindEmail, in, 2, "")
	empty.Subject = nil
	rows := []crmcontracts.Activity{withheld, empty}
	for i := range 20 {
		rows = append(rows, turnRow(crmcontracts.ActivityKindEmail, in, 3+i, strings.Repeat("long ", 500)))
	}
	got := draftcore.LanguageEvidenceOf(rows, turnsNow)
	// The reader's window holds LanguageEvidenceTurns turns, one of which is the empty row.
	if want := draftcore.LanguageEvidenceTurns - 1; len(got) != want {
		t.Errorf("got %d strings, want %d", len(got), want)
	}
	for _, text := range got {
		if strings.Contains(text, "secret") || len([]rune(text)) > draftcore.LanguageEvidenceRunes {
			t.Errorf("evidence leaked a withheld row or exceeded the rune bound: %d runes", len([]rune(text)))
		}
	}
}
