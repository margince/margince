// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package approvalsubject defines payload-bound responsibility shared by
// approval staging, inbox reads and webhook delivery.
package approvalsubject

import (
	"encoding/json"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// KindCounterparty names the import decision in both its writer and readers.
const KindCounterparty = "capture_counterparty"

// Counterparty carries the original import and the disposition acceptance must
// resolve. Ownership stays with that import, including older system-staged rows
// whose on_behalf_of column is empty.
type Counterparty struct {
	DispositionID ids.UUID `json:"disposition_id"`
	Email         string   `json:"email"`
	DisplayName   string   `json:"display_name"`
	Domain        string   `json:"domain"`
	OwnerID       ids.UUID `json:"owner_id"`
	ActivityID    ids.UUID `json:"activity_id"`
}

// PayloadOwnedKinds is the census of kinds whose persisted payload names their
// responsible member. Consumers derive their staging/read parity checks here.
func PayloadOwnedKinds() []string { return []string{KindCounterparty} }

// PayloadOwned distinguishes payload ownership from on_behalf_of ownership.
func PayloadOwned(kind string) bool { return slices.Contains(PayloadOwnedKinds(), kind) }

// Owner returns zero for absent or malformed ownership, which admits no reader.
func Owner(body json.RawMessage) ids.UUID {
	var proposal Counterparty
	if err := json.Unmarshal(body, &proposal); err != nil {
		return ids.Nil
	}
	return proposal.OwnerID
}

// Withheld applies the persisted responsibility to inbox and delivery readers.
func Withheld(kind string, body json.RawMessage, reader ids.UUID) bool {
	if !PayloadOwned(kind) {
		return false
	}
	owner := Owner(body)
	return owner.IsZero() || reader != owner
}
