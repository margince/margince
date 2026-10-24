// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// KindContactShare asks the owner of a contact a colleague cannot see to share
// it with that colleague. The colleague tried to create the same person.
const KindContactShare = "contact_share"

// ContactShare is that request. OwnerID decides it, under the key every
// payload-owned kind uses; RequesterID never reads it.
type ContactShare struct {
	ContactID   ids.UUID `json:"contact_id"`
	OwnerID     ids.UUID `json:"owner_id"`
	RequesterID ids.UUID `json:"requester_id"`
}
