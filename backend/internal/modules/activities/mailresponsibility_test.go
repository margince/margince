// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMailOwnerNamesOnlyAnUnambiguousDirectSeat(t *testing.T) {
	t.Parallel()
	first, second := ids.UUID{1}, ids.UUID{2}
	for _, tc := range []struct {
		name       string
		recipients []ids.UUID
		selected   *ids.UUID
		want       ids.UUID
	}{
		{name: "none"},
		{name: "sole", recipients: []ids.UUID{first}, want: first},
		{name: "two", recipients: []ids.UUID{first, second}},
		{name: "selected among two", recipients: []ids.UUID{first, second}, selected: &second, want: second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row := waitingReplyForOwner(WaitingReply{Kind: kindEmail, OwnerID: second, DirectRecipients: tc.recipients}, tc.selected)
			if row.OwnerID != tc.want || !row.OwnerKnown {
				t.Fatalf("owner=%v known=%v, want%v", row.OwnerID, row.OwnerKnown, tc.want)
			}
		})
	}
}
