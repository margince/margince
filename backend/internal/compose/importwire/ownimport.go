// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ownImportObjects are what a member who imports only their own records may
// import. Companies, deals and activities are workspace imports.
var ownImportObjects = []string{migration.ObjectLead, migration.ObjectContact}

// RefuseBeyondOwnImport refuses a mapping with a part a member importing their
// own records may not import, before anything is tested.
func RefuseBeyondOwnImport(ctx context.Context, mapping migration.RunMapping) error {
	if auth.WorkspaceImporter(ctx) {
		return nil
	}
	for _, part := range mapping.PartList() {
		if !slices.Contains(ownImportObjects, part.Object) {
			return httperr.Validation("mapping", "own_import_only",
				"you can import your own leads and contacts; ask an administrator to import "+part.Object+" records")
		}
	}
	return nil
}

// NotYourRecord refuses a row of an own import (own set) that reaches a
// record another seat owns, or no seat. Forcing the owner on every write would
// otherwise hand the importer a colleague's record through an id cell or a
// remembered ref. current is the stored record as its contract JSON.
func NotYourRecord(current []byte, own *ids.UserID) (migration.Reason, error) {
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
