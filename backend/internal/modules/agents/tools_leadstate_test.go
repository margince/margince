// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChangeStatePassesArgsThrough(t *testing.T) {
	_, err := fullRegistry(t).InvokeServed(agentCtx(), "change_state",
		json.RawMessage(`{"action":"promote","lead_id":"0192f5c4-6c1e-7a4a-9d36-1f1f0c3a2b11"}`))
	if err == nil || !strings.Contains(err.Error(), "trigger") {
		t.Fatalf("promote without a trigger answered %v, want promote_lead's refusal naming trigger", err)
	}
}
