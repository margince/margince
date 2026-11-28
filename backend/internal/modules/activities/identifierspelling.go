// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/platform/database/storekit"

const spellingActivityParent = "activity_id"

// IdentifierSpellings are the identifier columns this module owns.
func IdentifierSpellings() []storekit.IdentifierColumn {
	return []storekit.IdentifierColumn{
		{Table: "activity_participant", Column: "address", Object: linkEntityActivity, Parent: spellingActivityParent},
	}
}
