// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSettlementShowsWhetherOurReplyCarriedFiles(t *testing.T) {
	for _, files := range [][]string{nil, {"report.pdf", "prices.xlsx"}} {
		batch := []settleCandidate{{Messages: []threadMessage{{ID: ids.NewV7(), Direction: "outbound", Body: "The report is attached.", Attachments: files}}}}
		prompt := settleRequest(batch).Messages[0].Content
		want := "Attachments: 0"
		if len(files) > 0 {
			want = "Attachments: 2 | report.pdf | prices.xlsx"
		}
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt does not state %q: %s", want, prompt)
		}
	}
}
