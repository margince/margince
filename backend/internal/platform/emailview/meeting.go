// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import "strings"

// MeetingDescription leaves event time and location to the provider's live fields.
func MeetingDescription(description string) string {
	return strings.TrimSpace(description)
}
