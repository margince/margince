// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestARouterWithAViewDeclaresAShapeEveryTargetsAnswerSatisfies(t *testing.T) {
	slipping := `{"type":"object","required":["data"],"properties":{"data":{"type":"object","required":["deals"],"properties":{
		"deals":{"type":"array","items":{"type":"object","required":["deal_id","rank"],"properties":{"deal_id":{"type":"string"},"rank":{"type":"integer"}}}}}}}}`
	atRisk := `{"type":"object","required":["data"],"properties":{"data":{"type":"object","required":["deals","deals_scanned"],"properties":{
		"deals":{"type":"array","items":{"type":"object","required":["deal_id"],"properties":{"deal_id":{"type":"string"},"risks":{"type":"array"}}}},
		"deals_scanned":{"type":"integer"}}}}}`
	merged := targetsOutput("read_attention", []mcp.ToolSpec{
		{Name: "slipping", OutputSchema: json.RawMessage(slipping)},
		{Name: "at_risk", OutputSchema: json.RawMessage(atRisk)},
	})
	for _, answer := range []string{
		`{"data":{"deals":[{"deal_id":"d","rank":1}]}}`,
		`{"data":{"deals":[{"deal_id":"d","risks":[]}],"deals_scanned":3}}`,
	} {
		if defect := ResultDefect(merged, json.RawMessage(answer)); defect != "" {
			t.Errorf("a target's own answer %s fails the router's shape: %s", answer, defect)
		}
	}
	if defect := ResultDefect(merged, json.RawMessage(`{"data":{"deals":[{"rank":1}]}}`)); defect == "" {
		t.Error("a member every target requires is still required of the router's answer")
	}
}

func TestTargetsThatDisagreeOnAMembersTypeCannotShareAView(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a member typed differently by two targets was merged into one shape")
		}
	}()
	targetsOutput("read_attention", []mcp.ToolSpec{
		{Name: "a", OutputSchema: json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}}}`)},
		{Name: "b", OutputSchema: json.RawMessage(`{"type":"object","properties":{"n":{"type":"string"}}}`)},
	})
}
