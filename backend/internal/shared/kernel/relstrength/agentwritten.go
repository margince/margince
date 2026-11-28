// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const agentActivityOrigin = "agent"

// RecordedByAgent identifies an assistant through the activity origin or writer.
func RecordedByAgent(origin, capturedBy string) bool {
	return origin == agentActivityOrigin || strings.HasPrefix(capturedBy, string(principal.PrincipalAgent)+":")
}

// RecordedByAgentSQL is RecordedByAgent over an activity row.
func RecordedByAgentSQL(alias string) string {
	return "(" + alias + ".origin = '" + agentActivityOrigin + "' OR starts_with(coalesce(" + alias + ".captured_by, ''), '" + string(principal.PrincipalAgent) + ":'))"
}
