// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// CitedActivityID keeps citation joins on the UUID primary key rather than a text scan.
func CitedActivityID(item string) string {
	return CitedEntityID(item, "source_id")
}

// CitedEntityID parses a canonical UUID from a trusted JSON field name.
func CitedEntityID(item, field string) string {
	return "(CASE WHEN (" + item + "->>'" + field + "') ~ " +
		"'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' " +
		"THEN (" + item + "->>'" + field + "')::uuid END)"
}
