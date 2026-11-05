// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/modules/contacts"

// canonicalIndustry renders an imported industry the way the store holds it.
// A file that spells the industry another way is then no change. Text that
// names no code compares as written.
func canonicalIndustry(trimmed string) string {
	if code, ok := contacts.ResolveIndustry(trimmed); ok {
		return code
	}
	return trimmed
}
