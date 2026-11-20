// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"
)

func extTask(name Task, unit string, background bool) ExtensionTask {
	return ExtensionTask{
		Task: name, Unit: unit, DisplayName: "Shown", Summary: "Does a thing.",
		Ladder: []Tier{TierCheapCloud}, Background: background, Sites: []string{"one_shot"},
	}
}

func TestRegisteredTasksAreRoutedButNotCertified(t *testing.T) {
	t.Cleanup(ResetExtensionTasks)
	if err := RegisterExtensionTasks([]ExtensionTask{extTask("ext_a_ask", "a", false), extTask("ext_a_nightly", "a", true)}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(RoutedTasks(), Task("ext_a_ask")) {
		t.Error("RoutedTasks lacks the extension task")
	}
	if slices.Contains(AllTasks(), Task("ext_a_ask")) {
		t.Error("AllTasks lists an extension task, so certification would change")
	}
	if got := taskDisplayNames["ext_a_ask"]; got != "Shown" {
		t.Errorf("display name = %q", got)
	}
	if IsBackground("ext_a_ask") || !IsBackground("ext_a_nightly") {
		t.Error("execution modes are wrong")
	}
	if unit, ok := ExtensionTaskUnit("ext_a_ask"); !ok || unit != "a" {
		t.Errorf("unit = %q, %v", unit, ok)
	}
	ResetExtensionTasks()
	if slices.Contains(RoutedTasks(), Task("ext_a_ask")) {
		t.Error("Reset left the task routed")
	}
}

func TestARefusedTaskRegistersNothing(t *testing.T) {
	t.Cleanup(ResetExtensionTasks)
	badTier := extTask("ext_b_bad", "b", false)
	badTier.Ladder = []Tier{"galactic"}
	core := extTask(AllTasks()[0], "b", false)
	for name, bad := range map[string]ExtensionTask{"unknown tier": badTier, "core clash": core} {
		if err := RegisterExtensionTasks([]ExtensionTask{extTask("ext_b_ok", "b", false), bad}); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, ok := ExtensionTaskUnit("ext_b_ok"); ok {
			t.Errorf("%s: the valid task was applied", name)
		}
	}
	_ = RegisterExtensionTasks([]ExtensionTask{extTask("ext_b_ok", "b", false)})
	if err := RegisterExtensionTasks([]ExtensionTask{extTask("ext_b_ok", "c", false)}); err == nil {
		t.Error("a second unit took another unit's task")
	}
}
