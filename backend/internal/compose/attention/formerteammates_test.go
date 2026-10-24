// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A team view keeps the open work of a colleague who left, beside the work of
// the live team, and still drops a stranger's.
func TestATeamViewKeepsTheWorkOfAColleagueWhoLeft(t *testing.T) {
	live := ids.MustParse("01a05500-0000-7000-8000-0000000000a1")
	left := ids.MustParse("01a05500-0000-7000-8000-0000000000a2")
	stranger := ids.MustParse("01a05500-0000-7000-8000-0000000000a3")
	rows := []ranked{{ownerRef: ownedBy(live)}, {ownerRef: ownedBy(left)}, {ownerRef: ownedBy(stranger)}}

	kept := rowsForRoster(rows, []TeamMember{{UserID: live}}, left)

	owners := map[ids.UUID]bool{}
	for _, row := range kept {
		owners[row.ownerRef.user] = true
	}
	if !owners[live] || !owners[left] || owners[stranger] || len(kept) != 2 {
		t.Errorf("kept owners %v, want the live teammate and the one who left, not the stranger", owners)
	}
}
