// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// IdentifierSpellings are the identifier columns this module owns.
func IdentifierSpellings() []storekit.IdentifierColumn {
	return []storekit.IdentifierColumn{
		{Table: "deal_room_participant", Column: "email", Object: "deal_room", Parent: "room_id"},
	}
}
