// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// approvalsubject reads who a commitment is for by JSON key, because it cannot
// import this package. These marshal the real payload, so a renamed tag fails.
func TestACommitmentProposalNamesTheColleagueItIsFor(t *testing.T) {
	promiser, holder, other := ids.NewV7(), ids.NewV7(), ids.NewV7()
	task := ids.NewV7()
	cases := []struct {
		name     string
		proposal CommitmentTaskProposal
		want     *ids.UUID
	}{
		{"a new task for its promiser", CommitmentTaskProposal{SeatID: &promiser, PromiserSeatID: &promiser}, &promiser},
		{"recipients for the task's holder", CommitmentTaskProposal{ExistingTaskID: &task, SeatID: &holder}, &holder},
		{"an invited promiser's request staged before they had a seat", CommitmentTaskProposal{PromiserSeatID: &promiser}, &promiser},
		{"nobody named", CommitmentTaskProposal{}, nil},
		{"recipients for a task nobody holds", CommitmentTaskProposal{ExistingTaskID: &task, PromiserSeatID: &promiser}, nil},
		{"staged for somebody who did not promise", CommitmentTaskProposal{SeatID: &other, PromiserSeatID: &promiser}, nil},
		{"no promiser at all", CommitmentTaskProposal{SeatID: &other, PromiserUnresolved: true}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, err := json.Marshal(c.proposal)
			if err != nil {
				t.Fatal(err)
			}
			got, named := approvalsubject.CommitmentAskedOf(body)
			if c.want == nil {
				if named {
					t.Fatalf("the proposal names %s, want nobody", got)
				}
				return
			}
			if !named || got != *c.want {
				t.Fatalf("the proposal names %s (named %v), want %s", got, named, *c.want)
			}
			if approvalsubject.Withheld(CommitmentTaskKind, body, *c.want) {
				t.Error("the proposal is withheld from the colleague it is for")
			}
			if !approvalsubject.Withheld(CommitmentTaskKind, body, ids.NewV7()) {
				t.Error("the proposal is shown to a colleague it is not for")
			}
		})
	}
}
