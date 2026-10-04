// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A reviewer corrects a promise's wording and its day, and nothing else: the
// locator is the memory that stops a dismissed promise coming back, and the
// party and quote are what the reviewer agreed to.
func TestACommitmentEditReachesOnlyTheWordingAndTheDay(t *testing.T) {
	staged := CommitmentTaskProposal{
		SourceActivityID: ids.NewV7(), Summary: "Send the revised pricing", Party: "Priya",
		DueDate: "2026-09-08", Quote: "I'll send the revised pricing over by Friday.",
		Locator: "abc", Body: "Priya committed to this.",
	}
	for _, c := range []struct {
		name   string
		edit   func(p *CommitmentTaskProposal)
		refuse bool
	}{
		{"the wording", func(p *CommitmentTaskProposal) { p.Summary = "Send pricing v2" }, false},
		{"the day", func(p *CommitmentTaskProposal) { p.DueDate = "2026-09-09" }, false},
		{"clearing the day", func(p *CommitmentTaskProposal) { p.DueDate = "" }, false},
		{"a day that is not a date", func(p *CommitmentTaskProposal) { p.DueDate = "Friday" }, true},
		{"the locator", func(p *CommitmentTaskProposal) { p.Locator = "xyz" }, true},
		{"the party", func(p *CommitmentTaskProposal) { p.Party = "Dana" }, true},
		{"the quote", func(p *CommitmentTaskProposal) { p.Quote = "something else" }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			edited := staged
			c.edit(&edited)
			err := commitmentTaskPrecheck()(context.Background(), mustJSON(t, staged), mustJSON(t, edited))
			var invalid *approvals.InvalidEditError
			if refused := errors.As(err, &invalid); refused != c.refuse {
				t.Errorf("editing %s: refused=%v (err %v), want refused=%v", c.name, refused, err, c.refuse)
			}
		})
	}
}

func mustJSON(t *testing.T, v CommitmentTaskProposal) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
