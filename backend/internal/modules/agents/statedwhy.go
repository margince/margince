// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"strings"
)

// whyProperty is the argument a field-changing tool takes for the reason
// behind the change. The surface reads it from every tool that declares it:
// the audit rows of a call that runs keep it, and a staged call's card leads
// with it.
const whyProperty = `"why":{"type":"string","minLength":1,"maxLength":600,"description":"Why you are making this change, ` +
	`in one sentence the user reads beside it. Name the person, record or message and the fact that prompted it, ` +
	`for example: Anna Weber wrote on 14 Nov that the rollout moves to March."}`

// statedWhy is the call's own reason, trimmed, or "" when it gave none.
func statedWhy(args json.RawMessage) string {
	var call struct {
		Why string `json:"why"`
	}
	if json.Unmarshal(args, &call) != nil {
		return ""
	}
	return strings.TrimSpace(call.Why)
}
