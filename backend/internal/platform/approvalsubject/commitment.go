// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import (
	"encoding/json"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// KindCommitmentTask is work read from a conversation, staged for its named
// colleague to confirm.
const KindCommitmentTask = "commitment_task"

// commitmentSubject is the part of compose's CommitmentTaskProposal that says
// who it is for. The keys are that type's JSON tags; a compose test marshals
// the real type through CommitmentAskedOf, so a renamed key fails there.
type commitmentSubject struct {
	ReconfirmSource bool      `json:"reconfirm_source"`
	ExistingTaskID  *ids.UUID `json:"existing_task_id"`
	SeatID          *ids.UUID `json:"seat_id"`
	PromiserSeatID  *ids.UUID `json:"promiser_seat_id"`
	FollowUpOf      *ids.UUID `json:"follow_up_of"`
}

// CommitmentAskedOf binds review to its named colleague. A source question or
// customer follow-up names a holder without asserting that they made a promise.
func CommitmentAskedOf(body json.RawMessage) (ids.UUID, bool) {
	var subject commitmentSubject
	if err := json.Unmarshal(body, &subject); err != nil {
		return ids.Nil, false
	}
	if subject.ExistingTaskID != nil || subject.FollowUpOf != nil || subject.ReconfirmSource {
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

// commitmentOwnerRecordKeys are the payload keys that name the records a
// promise is filed under, the claims among them.
var commitmentOwnerRecordKeys = []string{"links", "claim_id", "claim_ids"}

// WithoutOwnersRecords removes from a commitment proposal read out of private
// mail the ids of the mailbox owner's records, for any reader but that owner.
// The colleague the promise is for may open the message, not those records,
// and an id would tell them which records exist. Other kinds pass unchanged.
func WithoutOwnersRecords(kind string, change map[string]any, reader ids.UUID) map[string]any {
	owner, _ := change["private_to"].(string)
	if kind != KindCommitmentTask || owner == "" || owner == reader.String() {
		return change
	}
	out := make(map[string]any, len(change))
	for key, value := range change {
		if !slices.Contains(commitmentOwnerRecordKeys, key) {
			out[key] = value
		}
	}
	return out
}
