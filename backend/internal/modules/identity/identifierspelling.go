// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// IdentifierSpellings are the identifier columns this module owns.
func IdentifierSpellings() []storekit.IdentifierColumn {
	return []storekit.IdentifierColumn{
		{Table: "app_user", Column: "email", Object: "user", Parent: "id"},
	}
}
