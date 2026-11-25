// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// RequiredCalls reads every key that requires a call, each alternative on its
// own. A pattern that spells an argument pins it, as a must_call_with does.
func TestRequiredCallsReadsEveryCallAScenarioRequires(t *testing.T) {
	dir := t.TempDir()
	body := "name: s\nmust_call:\n  # why\n  - a|b\nmust_call_with: [c.action=x|d.kind=y]\n" +
		"must_call_matching:\n  - 'e~\"action\": \"z\" || f~free text'\n  - g~\"owner_id\"\n" +
		"may_call: [h]\nprompt: |\n  must_call:\n  - not_a_tool\n"
	if err := os.WriteFile(filepath.Join(dir, "case1-s.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	calls, err := RequiredCalls(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := calls["s"]
	slices.Sort(got)
	want := []string{"a", "b", "c.action=x", "d.kind=y", "e", "e.action=z", "f", "g"}
	if !slices.Equal(got, want) {
		t.Errorf("RequiredCalls = %v, want %v", got, want)
	}
}
