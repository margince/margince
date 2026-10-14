// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"strconv"
	"testing"
)

func filledAgentAllowancePatch() AgentAllowancePatch {
	value := func(n int) *int { return &n }
	return AgentAllowancePatch{
		ReadsPerWindow: value(1), WritesPerWindow: value(2), EgressPerWindow: value(3), CallsPerWindow: value(4),
		WindowHours: value(5),
	}
}

func TestEveryAgentAllowanceCountWritesItsOwnSetting(t *testing.T) {
	writes, err := filledAgentAllowancePatch().writes()
	if err != nil {
		t.Fatalf("encoding a full patch: %v", err)
	}
	written := map[string]string{}
	for _, write := range writes {
		written[write.entry.Key()] = string(write.raw)
	}
	for key, want := range map[string]int{
		AgentReadsPerWindow.Key(): 1, AgentWritesPerWindow.Key(): 2,
		AgentEgressPerWindow.Key(): 3, AgentCallsPerWindow.Key(): 4,
	} {
		if got := written[key]; got != strconv.Itoa(want) {
			t.Errorf("%s was written %q, want %d", key, got, want)
		}
	}
	if raw, ok := written[AgentWindowHours.Key()]; ok {
		t.Errorf("the window length was written straight (%s); it must wait for the next reset", raw)
	}
}

func TestAnEmptyAgentAllowancePatchWritesNothing(t *testing.T) {
	writes, err := AgentAllowancePatch{}.writes()
	if err != nil {
		t.Fatalf("encoding an empty patch: %v", err)
	}
	for _, write := range writes {
		if write.raw != nil {
			t.Errorf("%s encoded %s from an absent field", write.entry.Key(), write.raw)
		}
	}
}
