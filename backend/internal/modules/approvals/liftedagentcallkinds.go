// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	_ "embed"
	"strings"
)

// liftedAgentCallKindList holds, one per line, the tool kinds that ran at once
// for an agent acting as its user when the withdrawal task shipped. It is
// frozen: a shipped task withdraws the same kinds on every upgrade, whatever
// waits since.
//
//go:embed liftedagentcallkinds.txt
var liftedAgentCallKindList string

// LiftedAgentCallKinds is the kinds the withdrawal task closes approvals for.
func LiftedAgentCallKinds() []string {
	return strings.Fields(liftedAgentCallKindList)
}
