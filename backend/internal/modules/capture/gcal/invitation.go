// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"strings"
)

// InvitationRequest recovers the stable request identity from a provider echo.
func InvitationRequest(eventID string, raw []byte) (string, error) {
	return strings.ReplaceAll(eventID, "-", ""), nil
}
