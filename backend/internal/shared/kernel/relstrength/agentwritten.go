// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordedByAgent includes legacy notes and passport calls without a scheduled run.
func RecordedByAgent(origin, capturedBy string) bool {
	return origin == "agent" || strings.HasPrefix(capturedBy, string(principal.PrincipalAgent)+":")
}

// RecordedByAgentSQL is RecordedByAgent over an activity row.
func RecordedByAgentSQL(alias string) string {
	return "(" + alias + ".origin = 'agent' OR starts_with(coalesce(" + alias + ".captured_by, ''), '" + string(principal.PrincipalAgent) + ":'))"
}
