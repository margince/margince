// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A draft the caller could not move is named in the removal preview, so the
// removal does not fail after the admin confirmed it.
func TestAnUnmovableDraftBlocksTheRemovalInItsPreview(t *testing.T) {
	receiver := ids.NewV7()
	left := retirementMember{Status: userStatusDeactivated}
	for name, transferable := range map[string]int{"movable": 1, "unmovable": 0} {
		work := MemberWorkInspection{
			Owned:        crmcontracts.HandoverCounts{MailDraft: 1},
			VisibleOwned: crmcontracts.HandoverCounts{MailDraft: 1},
			Transferable: crmcontracts.HandoverCounts{MailDraft: transferable},
		}
		var out crmcontracts.MemberRetirementPreview
		applyRetirementWork(&out, work, left, Identity{}, &receiver, false)
		if blocked := slices.Contains(out.Blockers, "work_not_transferable"); blocked != (transferable == 0) {
			t.Errorf("%s draft: blockers %v", name, out.Blockers)
		}
	}
}
