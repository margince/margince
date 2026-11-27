// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package claims

import "strings"

// SaysStock reports a sentence that leans on one of a site's stock phrases,
// copy that would fit any record and so explains none.
func SaysStock(text string, phrases []string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range phrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
