// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// log_activity's schema is closed, so a meeting outcome it does not declare is
// one an agent can never report while the web form always can.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLogActivityDeclaresTheContractsMeetingOutcomes(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(logActivity{}.Spec().InputSchema, &schema); err != nil {
		t.Fatalf("decoding the log_activity schema: %v", err)
	}
	field, ok := schema.Properties["meeting_status"]
	if !ok {
		t.Fatal("log_activity does not declare meeting_status")
	}
	if got := strings.Join(field.Enum, ","); got != "booked,held,no_show,canceled" {
		t.Fatalf("meeting_status enum = %s, want the contract's booked,held,no_show,canceled", got)
	}
}

// The tool forwards its arguments whole, so the outcome reaches the same
// strict decode against CreateActivityRequest that the REST create runs.
func TestLogActivityCarriesTheMeetingOutcomeToTheContractBody(t *testing.T) {
	s := idProbeDispatcher(t)
	ctx := scopedAgentCtx(principal.ScopeWrite)
	call := `{"name":"log_activity","arguments":{"kind":"meeting","subject":"Introductory lunch",` +
		`"occurred_at":"2026-03-09T12:00:00Z","meeting_status":"held"}}`

	answer := errorText(t, callMap(ctx, t, s, call))
	// The probe seam answers with errSeamReached only after the decode passed.
	if !strings.Contains(answer, internalFaultAdvice) {
		t.Fatalf("log_activity refused meeting_status before the seam: %q", answer)
	}
}
