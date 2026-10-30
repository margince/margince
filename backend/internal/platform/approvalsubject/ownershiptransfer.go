// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// KindOwnershipTransfer moves a contact or company from one colleague to
// another once both have agreed: one asks, the other accepts.
const KindOwnershipTransfer = "ownership_transfer"

// OwnershipTransfer is that move. FromID owns the record when it is staged and
// ToID receives it. RequestedBy is whichever of the two staged it, and DeciderID
// is always the other one.
type OwnershipTransfer struct {
	RecordType  string   `json:"record_type"`
	RecordID    ids.UUID `json:"record_id"`
	FromID      ids.UUID `json:"from_id"`
	ToID        ids.UUID `json:"to_id"`
	RequestedBy ids.UUID `json:"requested_by"`
	DeciderID   ids.UUID `json:"decider_id"`
	Note        string   `json:"note,omitempty"`
}

// OwnershipTransferDecider is the member a transfer is put to. It is zero when
// the decider also staged it, or when a party is neither the giver nor the
// receiver.
func OwnershipTransferDecider(body json.RawMessage) ids.UUID {
	var transfer OwnershipTransfer
	if err := json.Unmarshal(body, &transfer); err != nil {
		return ids.Nil
	}
	return transfer.decider()
}

func (t OwnershipTransfer) decider() ids.UUID {
	parties := t.FromID != t.ToID && !t.FromID.IsZero() && !t.ToID.IsZero()
	sides := (t.RequestedBy == t.FromID && t.DeciderID == t.ToID) ||
		(t.RequestedBy == t.ToID && t.DeciderID == t.FromID)
	if !parties || !sides {
		return ids.Nil
	}
	return t.DeciderID
}
