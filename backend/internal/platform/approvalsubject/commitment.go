// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// KindCommitmentTask is a promise one of our colleagues made, read out of a
// conversation and staged for them to confirm.
const KindCommitmentTask = "commitment_task"

// commitmentSubject is the part of compose's CommitmentTaskProposal that says
// who it is for. The keys are that type's JSON tags; a compose test marshals
// the real type through CommitmentAskedOf, so a renamed key fails there.
type commitmentSubject struct {
	ExistingTaskID *ids.UUID `json:"existing_task_id"`
	SeatID         *ids.UUID `json:"seat_id"`
	PromiserSeatID *ids.UUID `json:"promiser_seat_id"`
}

// CommitmentAskedOf is the one member a commitment proposal may be put to.
// A new task is for the colleague who made the promise: the payload must name
// them, and a seat it names must be theirs. A request to add recipients to an
// existing task is for the colleague who holds that task. Any other shape
// names nobody, and nobody may read, decide or be told about it.
func CommitmentAskedOf(body json.RawMessage) (ids.UUID, bool) {
	var subject commitmentSubject
	if err := json.Unmarshal(body, &subject); err != nil {
		return ids.Nil, false
	}
	if subject.ExistingTaskID != nil {
		return named(subject.SeatID)
	}
	if subject.SeatID != nil && (subject.PromiserSeatID == nil || *subject.SeatID != *subject.PromiserSeatID) {
		return ids.Nil, false
	}
	return named(subject.PromiserSeatID)
}

func named(seat *ids.UUID) (ids.UUID, bool) {
	if seat == nil || seat.IsZero() {
		return ids.Nil, false
	}
	return *seat, true
}
