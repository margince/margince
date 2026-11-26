// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workforce

// MemberWorkSnapshot keeps private dependency totals separate from visible work.
type MemberWorkSnapshot struct {
	Owned        int
	Visible      int
	Transferable int
	Stamp        []string
}
