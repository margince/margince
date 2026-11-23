// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// KindEditAccess asks the owner of a record a colleague can see but not change
// to share it with that colleague for writing. The owner stays the owner.
const KindEditAccess = "edit_access"

// EditAccess is that request. OwnerID decides it, under the key every
// payload-owned kind uses, and RequesterID receives the share.
type EditAccess struct {
	RecordType  string   `json:"record_type"`
	RecordID    ids.UUID `json:"record_id"`
	OwnerID     ids.UUID `json:"owner_id"`
	RequesterID ids.UUID `json:"requester_id"`
}

// EditAccessRequester is the colleague who asked, or zero for a malformed body.
func EditAccessRequester(body json.RawMessage) ids.UUID {
	var ask EditAccess
	if err := json.Unmarshal(body, &ask); err != nil {
		return ids.Nil
	}
	return ask.RequesterID
}
