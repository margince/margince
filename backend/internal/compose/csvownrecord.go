// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/migration"
)

// notYourRecord refuses a row of an own import that reaches a record another
// seat owns. Forcing the owner on every write would otherwise hand the
// importer a colleague's record through an id cell or a remembered ref.
func (w *csvWriters) notYourRecord(current []byte) (migration.Reason, error) {
	own := w.ownSeat()
	if own == nil {
		return migration.Reason{}, nil
	}
	var stored struct {
		OwnerID *string `json:"owner_id"`
	}
	if err := json.Unmarshal(current, &stored); err != nil {
		return migration.Reason{}, fmt.Errorf("import: reading the stored record's owner: %w", err)
	}
	if stored.OwnerID != nil && *stored.OwnerID == own.String() {
		return migration.Reason{}, nil
	}
	return migration.Why(migration.ReasonNotYourRecord), nil
}
