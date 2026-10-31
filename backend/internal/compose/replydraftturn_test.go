// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A colleague writing from one of our own domains arrives inbound. Read by
// direction, that row said the other side had spoken last, so the draft waited
// on a reply to our own message.
func TestAColleaguesInboundRowIsNotTheirTurn(t *testing.T) {
	t.Parallel()
	inbound := crmcontracts.ActivityDirectionInbound
	row := crmcontracts.Activity{Direction: &inbound}
	if !theirTurn(row) {
		t.Fatal("an inbound row with no voice reads as ours, want theirs")
	}
	row.Voice = &crmcontracts.ActivityVoice{
		Author: &crmcontracts.ActivityAuthor{Kind: crmcontracts.ActivityAuthorKindColleague},
	}
	if theirTurn(row) {
		t.Fatal("an inbound row a colleague wrote reads as theirs, want ours")
	}
}
