// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"
	"testing"
)

// A save that repeats what the automation holds changes nothing, whatever order
// the params keys arrive in; a save that moves any one of the three fields does.
func TestAnAutomationUpdateChangesNothingOnlyWhenEveryFieldRepeatsTheStoredOne(t *testing.T) {
	stored := Automation{
		Name: "Chase quotes", Enabled: false, Params: json.RawMessage(`{"days":3,"list":"a"}`),
	}
	same, other, on := "Chase quotes", "Chase offers", true
	off := false
	for name, tc := range map[string]struct {
		in   UpdateAutomationInput
		want bool
	}{
		"nothing sent":             {UpdateAutomationInput{}, true},
		"the stored name":          {UpdateAutomationInput{Name: &same}, true},
		"the stored state":         {UpdateAutomationInput{Enabled: &off}, true},
		"the stored params":        {UpdateAutomationInput{Params: map[string]any{"list": "a", "days": 3}}, true},
		"a new name":               {UpdateAutomationInput{Name: &other}, false},
		"enabling":                 {UpdateAutomationInput{Enabled: &on}, false},
		"other params":             {UpdateAutomationInput{Params: map[string]any{"days": 4, "list": "a"}}, false},
		"an unreadable stored row": {UpdateAutomationInput{Params: map[string]any{"days": 3}}, false},
	} {
		subject := stored
		if name == "an unreadable stored row" {
			subject.Params = json.RawMessage(`not json`)
		}
		if got := updateChangesNothing(subject, tc.in); got != tc.want {
			t.Errorf("%s: changes nothing = %t, want %t", name, got, tc.want)
		}
	}
}
