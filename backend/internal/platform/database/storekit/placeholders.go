// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/margince/margince/backend/internal/platform/database"

// Placeholders shares argument numbering with database-level callers.
func Placeholders[T any](arguments []T) string {
	return database.Placeholders(arguments)
}
